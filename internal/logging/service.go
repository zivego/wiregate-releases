package logging

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zivego/wiregate/internal/persistence/loggingrepo"
)

var (
	ErrInvalidSinkType  = errors.New("invalid log sink type")
	ErrInvalidTransport = errors.New("invalid syslog transport")
	ErrInvalidFormat    = errors.New("invalid syslog format")
	ErrSinkNotFound     = errors.New("log sink not found")
	ErrInvalidRouteRule = errors.New("invalid log route rule")
)

const (
	SinkTypeSyslog = "syslog"

	TransportUDP = "udp"
	TransportTCP = "tcp"
	TransportTLS = "tls"

	FormatRFC5424 = "rfc5424"
	FormatJSON    = "json"
)

var (
	allowedCategories = []string{"auth", "session", "user_mgmt", "policy", "agent", "enrollment", "reconcile", "security", "system"}
	allowedSeverities = []string{"debug", "info", "warn", "error"}
	redactedFields    = []string{
		"token",
		"session_token",
		"session_cookie",
		"cookie",
		"password",
		"private_key",
		"client_secret",
		"secret",
		"agent_auth_token",
		"enrollment_token",
		"raw_token",
	}
)

type Event struct {
	OccurredAt   time.Time      `json:"occurred_at"`
	Category     string         `json:"category"`
	Severity     string         `json:"severity"`
	Message      string         `json:"message"`
	Action       string         `json:"action,omitempty"`
	ResourceType string         `json:"resource_type,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	TestDelivery bool           `json:"test_delivery,omitempty"`
}

type SyslogConfig struct {
	Transport        string `json:"transport"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Format           string `json:"format"`
	Facility         int    `json:"facility"`
	AppName          string `json:"app_name"`
	HostnameOverride string `json:"hostname_override,omitempty"`
	CACertFile       string `json:"ca_cert_file,omitempty"`
	ClientCertFile   string `json:"client_cert_file,omitempty"`
	ClientKeyFile    string `json:"client_key_file,omitempty"`
}

type Sink struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Type      string       `json:"type"`
	Enabled   bool         `json:"enabled"`
	Syslog    SyslogConfig `json:"syslog"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type RouteRule struct {
	ID          string   `json:"id"`
	SinkID      string   `json:"sink_id"`
	Categories  []string `json:"categories"`
	MinSeverity string   `json:"min_severity"`
	Enabled     bool     `json:"enabled"`
}

type DeliveryStatus struct {
	SinkID              string     `json:"sink_id"`
	QueueDepth          int        `json:"queue_depth"`
	DroppedEvents       int        `json:"dropped_events"`
	TotalDelivered      int        `json:"total_delivered"`
	TotalFailed         int        `json:"total_failed"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastAttemptedAt     *time.Time `json:"last_attempted_at,omitempty"`
	LastDeliveredAt     *time.Time `json:"last_delivered_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type DeliveryFailure struct {
	ID           string         `json:"id"`
	SinkID       string         `json:"sink_id"`
	OccurredAt   time.Time      `json:"occurred_at"`
	Category     string         `json:"category"`
	Severity     string         `json:"severity"`
	Message      string         `json:"message"`
	Action       string         `json:"action,omitempty"`
	ErrorMessage string         `json:"error_message"`
	TestDelivery bool           `json:"test_delivery"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type StatusSnapshot struct {
	QueueCapacity     int               `json:"queue_capacity"`
	CurrentQueued     int               `json:"current_queued"`
	RedactedFields    []string          `json:"redacted_fields"`
	Sinks             []DeliveryStatus  `json:"sinks"`
	RecentFailures    []DeliveryFailure `json:"recent_failures"`
	DeadLetterCounts  map[string]int    `json:"-"`
}

type Options struct {
	QueueSize       int
	RetrySchedule   []time.Duration
	Now             func() time.Time
	DialTimeout     time.Duration
	DeadLetterLimit int
}

type deliveryTask struct {
	Sink  Sink
	Event Event
}

type Service struct {
	repo *loggingrepo.Repo
	log  *log.Logger
	now  func() time.Time

	queue         chan deliveryTask
	retrySchedule []time.Duration
	dialTimeout   time.Duration
	deadLetterLimit int

	mu     sync.RWMutex
	sinks  map[string]Sink
	routes []RouteRule

	statusMu sync.Mutex
	statuses map[string]DeliveryStatus

	closeOnce sync.Once
	workerWG  sync.WaitGroup
}

func NewService(repo *loggingrepo.Repo, logger *log.Logger, opts Options) *Service {
	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = 256
	}
	retrySchedule := opts.RetrySchedule
	if len(retrySchedule) == 0 {
		retrySchedule = []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 2 * time.Second
	}
	if logger == nil {
		logger = log.New(os.Stdout, "wiregate-log-export: ", log.LstdFlags|log.LUTC)
	}
	deadLetterLimit := opts.DeadLetterLimit
	if deadLetterLimit <= 0 {
		deadLetterLimit = 25
	}

	svc := &Service{
		repo:          repo,
		log:           logger,
		now:           now,
		queue:         make(chan deliveryTask, queueSize),
		retrySchedule: retrySchedule,
		dialTimeout:   dialTimeout,
		deadLetterLimit: deadLetterLimit,
		sinks:         map[string]Sink{},
		statuses:      map[string]DeliveryStatus{},
	}
	if repo != nil {
		if err := svc.reload(context.Background()); err != nil {
			logger.Printf("logging reload error: %v", err)
		}
		svc.workerWG.Add(1)
		go svc.worker()
	}
	return svc
}

func (s *Service) Emit(ctx context.Context, event Event) {
	if s == nil || s.repo == nil {
		return
	}
	normalized, err := normalizeEvent(event, s.now().UTC())
	if err != nil {
		s.log.Printf("logging emit normalization error: %v", err)
		return
	}
	sinks := s.matchingSinks(normalized)
	for _, sink := range sinks {
		task := deliveryTask{Sink: sink, Event: normalized}
		select {
		case s.queue <- task:
			s.adjustQueueDepth(ctx, sink.ID, 1)
		default:
			s.incrementDropped(ctx, sink.ID)
		}
	}
}

func (s *Service) ListSinks(ctx context.Context) ([]Sink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Sink, 0, len(s.sinks))
	for _, sink := range s.sinks {
		out = append(out, sink)
	}
	slices.SortFunc(out, func(a, b Sink) int { return strings.Compare(a.Name+a.ID, b.Name+b.ID) })
	return out, nil
}

func (s *Service) CreateSink(ctx context.Context, sink Sink) (Sink, error) {
	if s == nil || s.repo == nil {
		return Sink{}, ErrSinkNotFound
	}
	now := s.now().UTC()
	sink.ID = uuid.NewString()
	sink.CreatedAt = now
	sink.UpdatedAt = now
	if err := validateSink(sink); err != nil {
		return Sink{}, err
	}
	record, err := toSinkRecord(sink)
	if err != nil {
		return Sink{}, err
	}
	if err := s.repo.UpsertSink(ctx, record); err != nil {
		return Sink{}, err
	}
	s.ensureStatus(ctx, sink.ID)
	return sink, s.reload(ctx)
}

func (s *Service) UpdateSink(ctx context.Context, sink Sink) (Sink, error) {
	current, err := s.findSink(ctx, sink.ID)
	if err != nil {
		return Sink{}, err
	}
	if current == nil {
		return Sink{}, ErrSinkNotFound
	}
	sink.CreatedAt = current.CreatedAt
	sink.UpdatedAt = s.now().UTC()
	if err := validateSink(sink); err != nil {
		return Sink{}, err
	}
	record, err := toSinkRecord(sink)
	if err != nil {
		return Sink{}, err
	}
	if err := s.repo.UpsertSink(ctx, record); err != nil {
		return Sink{}, err
	}
	return sink, s.reload(ctx)
}

func (s *Service) DeleteSink(ctx context.Context, sinkID string) error {
	sink, err := s.findSink(ctx, sinkID)
	if err != nil {
		return err
	}
	if sink == nil {
		return ErrSinkNotFound
	}
	if err := s.repo.DeleteSink(ctx, sinkID); err != nil {
		return err
	}
	if err := s.repo.DeleteStatusBySinkID(ctx, sinkID); err != nil {
		return err
	}
	routes, err := s.GetRoutes(ctx)
	if err != nil {
		return err
	}
	filtered := make([]RouteRule, 0, len(routes))
	for _, route := range routes {
		if route.SinkID != sinkID {
			filtered = append(filtered, route)
		}
	}
	if _, err := s.UpdateRoutes(ctx, filtered); err != nil {
		return err
	}
	return s.reload(ctx)
}

func (s *Service) GetRoutes(ctx context.Context) ([]RouteRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]RouteRule(nil), s.routes...), nil
}

func (s *Service) UpdateRoutes(ctx context.Context, routes []RouteRule) ([]RouteRule, error) {
	if s == nil || s.repo == nil {
		return nil, ErrInvalidRouteRule
	}
	seen := map[string]struct{}{}
	for idx := range routes {
		if routes[idx].ID == "" {
			routes[idx].ID = uuid.NewString()
		}
		if _, ok := seen[routes[idx].ID]; ok {
			return nil, fmt.Errorf("%w: route ids must be unique", ErrInvalidRouteRule)
		}
		seen[routes[idx].ID] = struct{}{}
		if err := validateRouteRule(routes[idx], s.sinks); err != nil {
			return nil, err
		}
	}
	payload, err := json.Marshal(routes)
	if err != nil {
		return nil, fmt.Errorf("logging marshal routes: %w", err)
	}
	if err := s.repo.UpsertRoutes(ctx, loggingrepo.RouteConfig{
		ID:         "default",
		RoutesJSON: string(payload),
		UpdatedAt:  s.now().UTC(),
	}); err != nil {
		return nil, err
	}
	return routes, s.reload(ctx)
}

func (s *Service) GetStatus(ctx context.Context) (StatusSnapshot, error) {
	if s == nil || s.repo == nil {
		return StatusSnapshot{}, nil
	}
	s.statusMu.Lock()
	statuses := make([]DeliveryStatus, 0, len(s.statuses))
	for _, status := range s.statuses {
		statuses = append(statuses, status)
	}
	s.statusMu.Unlock()
	counts, err := s.repo.ListDeadLetterCounts(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}
	recent, err := s.repo.ListRecentDeadLetters(ctx, s.deadLetterLimit)
	if err != nil {
		return StatusSnapshot{}, err
	}
	slices.SortFunc(statuses, func(a, b DeliveryStatus) int { return strings.Compare(a.SinkID, b.SinkID) })
	return StatusSnapshot{
		QueueCapacity:    cap(s.queue),
		CurrentQueued:    len(s.queue),
		RedactedFields:   append([]string(nil), redactedFields...),
		Sinks:            statuses,
		RecentFailures:   mapDeliveryFailures(recent),
		DeadLetterCounts: counts,
	}, nil
}

func (s *Service) TestDelivery(ctx context.Context, sinkID string) error {
	event := Event{
		OccurredAt: s.now().UTC(),
		Category:   "system",
		Severity:   "info",
		Message:    "synthetic SIEM connectivity check",
		Action:     "logging.test_delivery",
		Metadata: map[string]any{
			"synthetic": true,
			"sink_id":   sinkID,
		},
		TestDelivery: true,
	}
	if strings.TrimSpace(sinkID) != "" {
		sink, err := s.findSink(ctx, sinkID)
		if err != nil {
			return err
		}
		if sink == nil {
			return ErrSinkNotFound
		}
		normalized, err := normalizeEvent(event, s.now().UTC())
		if err != nil {
			return err
		}
		select {
		case s.queue <- deliveryTask{Sink: *sink, Event: normalized}:
			s.adjustQueueDepth(ctx, sink.ID, 1)
			return nil
		default:
			s.incrementDropped(ctx, sink.ID)
			return nil
		}
	}
	s.Emit(ctx, event)
	return nil
}

func normalizeEvent(event Event, now time.Time) (Event, error) {
	category := strings.TrimSpace(event.Category)
	severity := strings.TrimSpace(event.Severity)
	if category == "" || !slices.Contains(allowedCategories, category) {
		return Event{}, fmt.Errorf("logging invalid category %q", category)
	}
	if severity == "" || severityRank(severity) < 0 {
		return Event{}, fmt.Errorf("logging invalid severity %q", severity)
	}
	event.Category = category
	event.Severity = severity
	event.Message = strings.TrimSpace(event.Message)
	if event.Message == "" {
		event.Message = category + " event"
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	event.Metadata = redactMap(event.Metadata)
	return event, nil
}

func validateSink(sink Sink) error {
	if strings.TrimSpace(sink.Name) == "" {
		return fmt.Errorf("%w: sink name is required", ErrInvalidSinkType)
	}
	if strings.TrimSpace(sink.Type) != SinkTypeSyslog {
		return fmt.Errorf("%w: only syslog is supported", ErrInvalidSinkType)
	}
	cfg := sink.Syslog
	if cfg.Transport != TransportUDP && cfg.Transport != TransportTCP && cfg.Transport != TransportTLS {
		return fmt.Errorf("%w: transport must be udp, tcp, or tls", ErrInvalidTransport)
	}
	if cfg.Format != FormatRFC5424 && cfg.Format != FormatJSON {
		return fmt.Errorf("%w: format must be rfc5424 or json", ErrInvalidFormat)
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("%w: syslog host is required", ErrInvalidSinkType)
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("%w: syslog port must be between 1 and 65535", ErrInvalidSinkType)
	}
	if cfg.Facility < 0 || cfg.Facility > 23 {
		return fmt.Errorf("%w: syslog facility must be between 0 and 23", ErrInvalidSinkType)
	}
	if cfg.Transport != TransportTLS && (cfg.CACertFile != "" || cfg.ClientCertFile != "" || cfg.ClientKeyFile != "") {
		return fmt.Errorf("%w: TLS file references require tls transport", ErrInvalidTransport)
	}
	if (cfg.ClientCertFile == "") != (cfg.ClientKeyFile == "") {
		return fmt.Errorf("%w: both client cert and client key files are required together", ErrInvalidTransport)
	}
	return nil
}

func validateRouteRule(rule RouteRule, sinks map[string]Sink) error {
	if strings.TrimSpace(rule.SinkID) == "" {
		return fmt.Errorf("%w: sink_id is required", ErrInvalidRouteRule)
	}
	if _, ok := sinks[rule.SinkID]; !ok {
		return fmt.Errorf("%w: sink_id %q does not exist", ErrInvalidRouteRule, rule.SinkID)
	}
	if severityRank(rule.MinSeverity) < 0 {
		return fmt.Errorf("%w: min_severity is invalid", ErrInvalidRouteRule)
	}
	if len(rule.Categories) == 0 {
		return fmt.Errorf("%w: at least one category is required", ErrInvalidRouteRule)
	}
	for _, category := range rule.Categories {
		if !slices.Contains(allowedCategories, category) {
			return fmt.Errorf("%w: category %q is invalid", ErrInvalidRouteRule, category)
		}
	}
	return nil
}

func (s *Service) reload(ctx context.Context) error {
	sinks, err := s.repo.ListSinks(ctx)
	if err != nil {
		return err
	}
	routeConfig, err := s.repo.GetRoutes(ctx)
	if err != nil {
		return err
	}
	var routes []RouteRule
	if strings.TrimSpace(routeConfig.RoutesJSON) != "" {
		if err := json.Unmarshal([]byte(routeConfig.RoutesJSON), &routes); err != nil {
			return fmt.Errorf("logging unmarshal routes: %w", err)
		}
	}
	sinkMap := make(map[string]Sink, len(sinks))
	for _, sink := range sinks {
		decoded, err := fromSinkRecord(sink)
		if err != nil {
			return err
		}
		sinkMap[decoded.ID] = decoded
	}
	s.mu.Lock()
	s.sinks = sinkMap
	s.routes = routes
	s.mu.Unlock()

	statuses, err := s.repo.ListStatuses(ctx)
	if err != nil {
		return err
	}
	s.statusMu.Lock()
	for _, status := range statuses {
		s.statuses[status.SinkID] = fromStatusRecord(status)
	}
	s.statusMu.Unlock()
	for sinkID := range sinkMap {
		s.ensureStatus(ctx, sinkID)
	}
	return nil
}

func (s *Service) findSink(ctx context.Context, sinkID string) (*Sink, error) {
	s.mu.RLock()
	if sink, ok := s.sinks[sinkID]; ok {
		s.mu.RUnlock()
		return &sink, nil
	}
	s.mu.RUnlock()
	record, err := s.repo.FindSinkByID(ctx, sinkID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	decoded, err := fromSinkRecord(*record)
	if err != nil {
		return nil, err
	}
	return &decoded, nil
}

func (s *Service) matchingSinks(event Event) []Sink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Sink
	seen := map[string]struct{}{}
	for _, route := range s.routes {
		if !route.Enabled || severityRank(event.Severity) < severityRank(route.MinSeverity) || !slices.Contains(route.Categories, event.Category) {
			continue
		}
		sink, ok := s.sinks[route.SinkID]
		if !ok || !sink.Enabled {
			continue
		}
		if _, exists := seen[sink.ID]; exists {
			continue
		}
		seen[sink.ID] = struct{}{}
		out = append(out, sink)
	}
	return out
}

func (s *Service) worker() {
	defer s.workerWG.Done()
	for task := range s.queue {
		s.adjustQueueDepth(context.Background(), task.Sink.ID, -1)
		s.deliverWithRetry(context.Background(), task)
	}
}

func (s *Service) deliverWithRetry(ctx context.Context, task deliveryTask) {
	var lastErr error
	for attempt := 0; attempt <= len(s.retrySchedule); attempt++ {
		lastErr = s.deliverSyslog(task.Sink, task.Event)
		if lastErr == nil {
			s.recordDeliverySuccess(ctx, task.Sink.ID)
			return
		}
		if attempt < len(s.retrySchedule) {
			time.Sleep(s.retrySchedule[attempt])
		}
	}
	s.recordDeliveryFailure(ctx, task.Sink.ID, task.Event, lastErr)
}

func (s *Service) deliverSyslog(sink Sink, event Event) error {
	if sink.Type != SinkTypeSyslog {
		return ErrInvalidSinkType
	}
	message, err := buildSyslogMessage(sink, event)
	if err != nil {
		return err
	}
	address := net.JoinHostPort(sink.Syslog.Host, strconv.Itoa(sink.Syslog.Port))
	switch sink.Syslog.Transport {
	case TransportUDP, TransportTCP:
		conn, err := net.DialTimeout(sink.Syslog.Transport, address, s.dialTimeout)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write([]byte(message))
		return err
	case TransportTLS:
		config, err := buildTLSConfig(sink.Syslog)
		if err != nil {
			return err
		}
		dialer := &net.Dialer{Timeout: s.dialTimeout}
		conn, err := tls.DialWithDialer(dialer, "tcp", address, config)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write([]byte(message))
		return err
	default:
		return ErrInvalidTransport
	}
}

func buildSyslogMessage(sink Sink, event Event) (string, error) {
	hostname := sink.Syslog.HostnameOverride
	if hostname == "" {
		value, err := os.Hostname()
		if err == nil && strings.TrimSpace(value) != "" {
			hostname = value
		} else {
			hostname = "wiregate"
		}
	}
	appName := sink.Syslog.AppName
	if appName == "" {
		appName = "wiregate"
	}
	priority := sink.Syslog.Facility*8 + severityToSyslogCode(event.Severity)
	body := event.Message
	if sink.Syslog.Format == FormatJSON {
		payload, err := json.Marshal(event)
		if err != nil {
			return "", fmt.Errorf("logging marshal syslog payload: %w", err)
		}
		body = string(payload)
	}
	return fmt.Sprintf("<%d>1 %s %s %s - - - %s\n", priority, event.OccurredAt.UTC().Format(time.RFC3339Nano), hostname, appName, body), nil
}

func buildTLSConfig(cfg SyslogConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: cfg.Host,
	}
	if cfg.CACertFile != "" {
		raw, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("logging read ca cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(raw) {
			return nil, fmt.Errorf("logging parse ca cert")
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.ClientCertFile != "" && cfg.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("logging load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func severityRank(value string) int {
	switch value {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	default:
		return -1
	}
}

func severityToSyslogCode(value string) int {
	switch value {
	case "error":
		return 3
	case "warn":
		return 4
	case "info":
		return 6
	default:
		return 7
	}
}

func redactMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		if shouldRedactKey(key) {
			out[key] = "[REDACTED]"
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			out[key] = redactMap(typed)
		case []any:
			out[key] = redactSlice(typed)
		default:
			out[key] = value
		}
	}
	return out
}

func redactSlice(input []any) []any {
	out := make([]any, 0, len(input))
	for _, value := range input {
		switch typed := value.(type) {
		case map[string]any:
			out = append(out, redactMap(typed))
		case []any:
			out = append(out, redactSlice(typed))
		default:
			out = append(out, value)
		}
	}
	return out
}

func shouldRedactKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, field := range redactedFields {
		if normalized == field || strings.Contains(normalized, field) {
			return true
		}
	}
	return false
}

func (s *Service) adjustQueueDepth(ctx context.Context, sinkID string, delta int) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	status := s.statuses[sinkID]
	status.SinkID = sinkID
	status.QueueDepth = maxInt(status.QueueDepth+delta, 0)
	status.UpdatedAt = s.now().UTC()
	s.statuses[sinkID] = status
	_ = s.repo.UpsertStatus(ctx, toStatusRecord(status))
}

func (s *Service) incrementDropped(ctx context.Context, sinkID string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	status := s.statuses[sinkID]
	status.SinkID = sinkID
	status.DroppedEvents++
	status.UpdatedAt = s.now().UTC()
	s.statuses[sinkID] = status
	_ = s.repo.UpsertStatus(ctx, toStatusRecord(status))
}

func (s *Service) recordDeliverySuccess(ctx context.Context, sinkID string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	now := s.now().UTC()
	status := s.statuses[sinkID]
	status.SinkID = sinkID
	status.TotalDelivered++
	status.ConsecutiveFailures = 0
	status.LastError = ""
	status.LastAttemptedAt = &now
	status.LastDeliveredAt = &now
	status.UpdatedAt = now
	s.statuses[sinkID] = status
	_ = s.repo.UpsertStatus(ctx, toStatusRecord(status))
}

func (s *Service) recordDeliveryFailure(ctx context.Context, sinkID string, event Event, err error) {
	s.statusMu.Lock()
	now := s.now().UTC()
	status := s.statuses[sinkID]
	status.SinkID = sinkID
	status.TotalFailed++
	status.ConsecutiveFailures++
	status.LastAttemptedAt = &now
	status.LastError = err.Error()
	status.UpdatedAt = now
	s.statuses[sinkID] = status
	s.statusMu.Unlock()
	_ = s.repo.UpsertStatus(ctx, toStatusRecord(status))
	if s.repo == nil {
		return
	}
	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		s.log.Printf("logging marshal dead letter payload error: %v", marshalErr)
		return
	}
	if insertErr := s.repo.InsertDeadLetter(ctx, loggingrepo.DeadLetter{
		ID:           uuid.NewString(),
		SinkID:       sinkID,
		OccurredAt:   event.OccurredAt.UTC(),
		EventJSON:    string(payload),
		ErrorMessage: err.Error(),
		TestDelivery: event.TestDelivery,
		CreatedAt:    now,
	}); insertErr != nil {
		s.log.Printf("logging insert dead letter error: %v", insertErr)
		return
	}
	if pruneErr := s.repo.PruneDeadLettersBySink(ctx, sinkID, s.deadLetterLimit); pruneErr != nil {
		s.log.Printf("logging prune dead letters error: %v", pruneErr)
	}
}

func (s *Service) ensureStatus(ctx context.Context, sinkID string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if _, ok := s.statuses[sinkID]; ok {
		return
	}
	now := s.now().UTC()
	status := DeliveryStatus{SinkID: sinkID, UpdatedAt: now}
	s.statuses[sinkID] = status
	_ = s.repo.UpsertStatus(ctx, toStatusRecord(status))
}

func toSinkRecord(sink Sink) (loggingrepo.Sink, error) {
	payload, err := json.Marshal(sink.Syslog)
	if err != nil {
		return loggingrepo.Sink{}, fmt.Errorf("logging marshal sink: %w", err)
	}
	return loggingrepo.Sink{
		ID:         sink.ID,
		Name:       sink.Name,
		Type:       sink.Type,
		Enabled:    sink.Enabled,
		ConfigJSON: string(payload),
		CreatedAt:  sink.CreatedAt,
		UpdatedAt:  sink.UpdatedAt,
	}, nil
}

func fromSinkRecord(record loggingrepo.Sink) (Sink, error) {
	var cfg SyslogConfig
	if err := json.Unmarshal([]byte(record.ConfigJSON), &cfg); err != nil {
		return Sink{}, fmt.Errorf("logging unmarshal sink config: %w", err)
	}
	return Sink{
		ID:        record.ID,
		Name:      record.Name,
		Type:      record.Type,
		Enabled:   record.Enabled,
		Syslog:    cfg,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, nil
}

func toStatusRecord(status DeliveryStatus) loggingrepo.DeliveryStatus {
	return loggingrepo.DeliveryStatus{
		SinkID:              status.SinkID,
		QueueDepth:          status.QueueDepth,
		DroppedEvents:       status.DroppedEvents,
		TotalDelivered:      status.TotalDelivered,
		TotalFailed:         status.TotalFailed,
		ConsecutiveFailures: status.ConsecutiveFailures,
		LastAttemptedAt:     status.LastAttemptedAt,
		LastDeliveredAt:     status.LastDeliveredAt,
		LastError:           status.LastError,
		UpdatedAt:           status.UpdatedAt,
	}
}

func fromStatusRecord(record loggingrepo.DeliveryStatus) DeliveryStatus {
	return DeliveryStatus{
		SinkID:              record.SinkID,
		QueueDepth:          record.QueueDepth,
		DroppedEvents:       record.DroppedEvents,
		TotalDelivered:      record.TotalDelivered,
		TotalFailed:         record.TotalFailed,
		ConsecutiveFailures: record.ConsecutiveFailures,
		LastAttemptedAt:     record.LastAttemptedAt,
		LastDeliveredAt:     record.LastDeliveredAt,
		LastError:           record.LastError,
		UpdatedAt:           record.UpdatedAt,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func RedactedFields() []string {
	return append([]string(nil), redactedFields...)
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.queue != nil {
			close(s.queue)
		}
		s.workerWG.Wait()
	})
}

func mapDeliveryFailures(records []loggingrepo.DeadLetter) []DeliveryFailure {
	out := make([]DeliveryFailure, 0, len(records))
	for _, record := range records {
		failure := DeliveryFailure{
			ID:           record.ID,
			SinkID:       record.SinkID,
			OccurredAt:   record.OccurredAt,
			ErrorMessage: record.ErrorMessage,
			TestDelivery: record.TestDelivery,
		}
		var event Event
		if err := json.Unmarshal([]byte(record.EventJSON), &event); err == nil {
			failure.Category = event.Category
			failure.Severity = event.Severity
			failure.Message = event.Message
			failure.Action = event.Action
			failure.Metadata = event.Metadata
		}
		out = append(out, failure)
	}
	return out
}

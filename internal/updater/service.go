package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zivego/wiregate/internal/version"
)

// Manifest is the remote version manifest fetched from the update URL.
type Manifest struct {
	LatestVersion string `json:"latest_version"`
	ReleasedAt    string `json:"released_at,omitempty"`
	ChangelogURL  string `json:"changelog_url,omitempty"`
}

// CheckResult is the response for a version check.
type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleasedAt      string `json:"released_at,omitempty"`
	ChangelogURL    string `json:"changelog_url,omitempty"`
	CheckedAt       string `json:"checked_at"`
}

// Status represents the current state of an update operation.
type Status struct {
	State     string `json:"state"` // idle | checking | pulling | restarting | failed
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

// Config holds updater configuration.
type Config struct {
	ManifestURL string
	SocketPath  string // Docker socket path, default /var/run/docker.sock
}

// Service manages server self-updates via the Docker Engine API.
type Service struct {
	cfg    Config
	logger *log.Logger
	docker *http.Client

	mu     sync.Mutex
	status Status
}

// NewService creates an updater service.
func NewService(cfg Config, logger *log.Logger) *Service {
	if cfg.SocketPath == "" {
		cfg.SocketPath = "/var/run/docker.sock"
	}
	// HTTP client that talks to Docker daemon via Unix socket.
	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", cfg.SocketPath)
			},
		},
		Timeout: 5 * time.Minute,
	}
	return &Service{
		cfg:    cfg,
		logger: logger,
		docker: dockerClient,
		status: Status{State: "idle"},
	}
}

// CheckForUpdate fetches the remote manifest and compares versions.
func (s *Service) CheckForUpdate(ctx context.Context) (*CheckResult, error) {
	s.mu.Lock()
	s.status = Status{State: "checking"}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.status.State == "checking" {
			s.status = Status{State: "idle"}
		}
		s.mu.Unlock()
	}()

	if s.cfg.ManifestURL == "" {
		return nil, fmt.Errorf("update manifest URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.ManifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "wiregate-updater/"+version.Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest returned %d", resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	current := version.Version
	latest := manifest.LatestVersion
	available := latest != "" && semverNewer(latest, current)

	return &CheckResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: available,
		ReleasedAt:      manifest.ReleasedAt,
		ChangelogURL:    manifest.ChangelogURL,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func semverNewer(a, b string) bool {
	aSemver, aOK := parseSemver(a)
	bSemver, bOK := parseSemver(b)
	if !aOK || !bOK {
		return strings.TrimSpace(a) != strings.TrimSpace(b)
	}
	if aSemver[0] != bSemver[0] {
		return aSemver[0] > bSemver[0]
	}
	if aSemver[1] != bSemver[1] {
		return aSemver[1] > bSemver[1]
	}
	return aSemver[2] > bSemver[2]
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	clean := strings.TrimSpace(v)
	clean = strings.TrimPrefix(clean, "v")
	clean = strings.TrimPrefix(clean, "V")
	parts := strings.Split(clean, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i := range parts {
		if parts[i] == "" {
			return out, false
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// StartUpdate begins the async update process. Returns immediately.
func (s *Service) StartUpdate(ctx context.Context, targetVersion string) error {
	s.mu.Lock()
	if s.status.State != "idle" && s.status.State != "failed" {
		s.mu.Unlock()
		return fmt.Errorf("update already in progress (state: %s)", s.status.State)
	}
	s.status = Status{
		State:     "pulling",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Unlock()

	go s.runUpdate(targetVersion)
	return nil
}

// Status returns the current update state.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) setStatus(state, message string) {
	s.mu.Lock()
	s.status.State = state
	s.status.Message = message
	s.mu.Unlock()
}

type containerMount struct {
	Source      string
	Destination string
}

// containerInfo holds the info we need from Docker inspect.
type containerInfo struct {
	ID     string
	Name   string
	Image  string
	Labels map[string]string
	Mounts []containerMount
}

type composeContext struct {
	Project     string
	WorkingDir  string
	ConfigFiles []string
	EnvFile     string
}

// selfInspect discovers this container's ID and image via the Docker API.
func (s *Service) selfInspect() (*containerInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
	}

	resp, err := s.docker.Get("http://localhost/v1.45/containers/" + hostname + "/json")
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inspect returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Image  string `json:"Image"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		Mounts []struct {
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode inspect: %w", err)
	}

	imageName := strings.TrimSpace(result.Config.Image)
	if imageName == "" {
		imageName = strings.TrimSpace(result.Image)
	}

	mounts := make([]containerMount, 0, len(result.Mounts))
	for _, m := range result.Mounts {
		mounts = append(mounts, containerMount{Source: m.Source, Destination: m.Destination})
	}

	return &containerInfo{
		ID:     result.ID,
		Name:   strings.TrimPrefix(result.Name, "/"),
		Image:  imageName,
		Labels: result.Config.Labels,
		Mounts: mounts,
	}, nil
}

func resolveTargetImage(currentImage, targetVersion string) (string, error) {
	image := strings.TrimSpace(currentImage)
	version := strings.TrimSpace(targetVersion)
	if image == "" {
		return "", fmt.Errorf("current image reference is empty")
	}
	if version == "" {
		return "", fmt.Errorf("target version is empty")
	}
	if !strings.Contains(image, "/") {
		return "", fmt.Errorf("image %q is not a registry/repository reference", image)
	}

	withoutDigest := image
	if i := strings.IndexRune(withoutDigest, '@'); i >= 0 {
		withoutDigest = withoutDigest[:i]
	}
	lastSlash := strings.LastIndex(withoutDigest, "/")
	lastColon := strings.LastIndex(withoutDigest, ":")
	repo := withoutDigest
	if lastColon > lastSlash {
		repo = withoutDigest[:lastColon]
	}
	if repo == "" {
		return "", fmt.Errorf("cannot resolve repository from image %q", image)
	}
	return repo + ":" + version, nil
}

func parseComposeContext(labels map[string]string) (composeContext, bool, error) {
	if len(labels) == 0 {
		return composeContext{}, false, nil
	}
	project := strings.TrimSpace(labels["com.docker.compose.project"])
	if project == "" {
		return composeContext{}, false, nil
	}

	cfgRaw := strings.TrimSpace(labels["com.docker.compose.project.config_files"])
	configFiles := splitCommaSeparated(cfgRaw)
	if len(configFiles) == 0 {
		return composeContext{}, true, fmt.Errorf("missing compose config files label")
	}

	workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	if workingDir == "" {
		workingDir = filepath.Dir(configFiles[0])
	}

	return composeContext{
		Project:     project,
		WorkingDir:  workingDir,
		ConfigFiles: configFiles,
		EnvFile:     strings.TrimSpace(labels["com.docker.compose.project.environment_file"]),
	}, true, nil
}

func splitCommaSeparated(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pullImage pulls the given image via Docker API.
func (s *Service) pullImage(ctx context.Context, image string) error {
	pullURL := fmt.Sprintf("http://localhost/v1.45/images/create?fromImage=%s", url.QueryEscape(image))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, nil)
	if err != nil {
		return fmt.Errorf("create pull request: %w", err)
	}
	if authHeader := registryAuthFromConfig(image); authHeader != "" {
		req.Header.Set("X-Registry-Auth", authHeader)
	}

	resp, err := s.docker.Do(req)
	if err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	streamErr := dockerStreamErrorMessage(body)
	if resp.StatusCode == http.StatusOK && streamErr == "" {
		return nil
	}

	detail := streamErr
	if detail == "" {
		detail = dockerAPIMessage(body)
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}

	return fmt.Errorf("%s", explainPullFailure(resp.StatusCode, image, detail))
}

type dockerPullEvent struct {
	Error       string `json:"error"`
	Message     string `json:"message"`
	ErrorDetail struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
}

func dockerStreamErrorMessage(body []byte) string {
	lines := strings.Split(string(body), "\n")
	lastErr := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var ev dockerPullEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if msg := strings.TrimSpace(ev.ErrorDetail.Message); msg != "" {
			lastErr = msg
			continue
		}
		if msg := strings.TrimSpace(ev.Error); msg != "" {
			lastErr = msg
		}
	}
	return lastErr
}

func dockerAPIMessage(body []byte) string {
	var ev dockerPullEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return ""
	}
	if msg := strings.TrimSpace(ev.ErrorDetail.Message); msg != "" {
		return msg
	}
	if msg := strings.TrimSpace(ev.Error); msg != "" {
		return msg
	}
	return strings.TrimSpace(ev.Message)
}

func explainPullFailure(statusCode int, image, detail string) string {
	base := strings.TrimSpace(detail)
	if base == "" {
		if statusCode == http.StatusOK {
			base = "registry returned an unspecified error"
		} else {
			base = fmt.Sprintf("registry returned HTTP %d", statusCode)
		}
	}

	hint := ""
	lower := strings.ToLower(base)
	switch {
	case statusCode == http.StatusNotFound || strings.Contains(lower, "manifest unknown") || strings.Contains(lower, "not found"):
		hint = "tag is not published in registry, mistyped, or package is private for this host"
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden || strings.Contains(lower, "denied") || strings.Contains(lower, "unauthorized"):
		hint = "registry authentication failed; verify GHCR token scopes and package visibility"
	}

	if hint != "" {
		return fmt.Sprintf("pull failed for %s: %s (HTTP %d). %s.", image, base, statusCode, hint)
	}
	if statusCode == http.StatusOK {
		return fmt.Sprintf("pull failed for %s: %s.", image, base)
	}
	return fmt.Sprintf("pull failed for %s: %s (HTTP %d).", image, base, statusCode)
}

func discoverComposeServiceImage(ctx context.Context, client *http.Client, project, service string) (string, error) {
	filters := map[string][]string{
		"label": {
			"com.docker.compose.project=" + project,
			"com.docker.compose.service=" + service,
		},
	}
	encoded, err := json.Marshal(filters)
	if err != nil {
		return "", fmt.Errorf("encode container filters: %w", err)
	}
	endpoint := "http://localhost/v1.45/containers/json?all=1&filters=" + url.QueryEscape(string(encoded))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create container list request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list containers returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var containers []struct {
		Image string `json:"Image"`
	}
	if err := json.Unmarshal(body, &containers); err != nil {
		return "", fmt.Errorf("decode container list: %w", err)
	}
	if len(containers) == 0 {
		return "", nil
	}
	return strings.TrimSpace(containers[0].Image), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	return strings.Join(quoted, " ")
}

func buildComposeArgs(meta composeContext) []string {
	args := []string{"compose", "-p", meta.Project, "--project-directory", meta.WorkingDir}
	for _, cfg := range meta.ConfigFiles {
		args = append(args, "-f", cfg)
	}
	if meta.EnvFile != "" {
		args = append(args, "--env-file", meta.EnvFile)
	}
	args = append(args, "-f", "/tmp/wiregate-update.override.yml")
	return args
}

func buildComposeUpdateScript(meta composeContext, backendImage, frontendImage string) string {
	services := []string{"backend"}
	var overrideBuilder strings.Builder
	overrideBuilder.WriteString("services:\n")
	overrideBuilder.WriteString("  backend:\n")
	overrideBuilder.WriteString("    image: " + backendImage + "\n")
	if frontendImage != "" {
		services = append(services, "frontend")
		overrideBuilder.WriteString("  frontend:\n")
		overrideBuilder.WriteString("    image: " + frontendImage + "\n")
	}

	composeArgs := buildComposeArgs(meta)
	pullCmd := shellJoin(append(append([]string{"docker"}, composeArgs...), append([]string{"pull"}, services...)...))
	upCmd := shellJoin(append(append([]string{"docker"}, composeArgs...), append([]string{"up", "-d"}, services...)...))

	var script strings.Builder
	script.WriteString("set -eu\n")
	script.WriteString("cat > /tmp/wiregate-update.override.yml <<'YAML'\n")
	script.WriteString(overrideBuilder.String())
	script.WriteString("YAML\n")
	script.WriteString(pullCmd + "\n")
	script.WriteString(upCmd + "\n")
	return script.String()
}

func findSocketSourceMount(mounts []containerMount) string {
	for _, m := range mounts {
		if strings.TrimSpace(m.Destination) == "/var/run/docker.sock" && strings.TrimSpace(m.Source) != "" {
			return strings.TrimSpace(m.Source)
		}
	}
	return "/var/run/docker.sock"
}

func helperBinds(meta composeContext, mounts []containerMount) []string {
	dirs := map[string]struct{}{}
	addDir := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		dir := filepath.Dir(path)
		if dir != "" {
			dirs[dir] = struct{}{}
		}
	}

	dirs[strings.TrimSpace(meta.WorkingDir)] = struct{}{}
	for _, cfg := range meta.ConfigFiles {
		addDir(cfg)
	}
	addDir(meta.EnvFile)

	binds := []string{fmt.Sprintf("%s:/var/run/docker.sock:rw", findSocketSourceMount(mounts))}
	// Forward docker config for registry auth in helper container.
	for _, m := range mounts {
		dest := strings.TrimSpace(m.Destination)
		if strings.HasSuffix(dest, "/config.json") && strings.Contains(dest, "docker") {
			binds = append(binds, fmt.Sprintf("%s:/root/.docker/config.json:ro", strings.TrimSpace(m.Source)))
			break
		}
	}
	dirList := make([]string, 0, len(dirs))
	for dir := range dirs {
		if strings.TrimSpace(dir) != "" {
			dirList = append(dirList, dir)
		}
	}
	sort.Strings(dirList)
	for _, dir := range dirList {
		binds = append(binds, fmt.Sprintf("%s:%s:ro", dir, dir))
	}
	return binds
}

func (s *Service) createContainer(ctx context.Context, name string, config map[string]any) (string, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode create payload: %w", err)
	}

	endpoint := "http://localhost/v1.45/containers/create?name=" + url.QueryEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("create create-container request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.docker.Do(req)
	if err != nil {
		return "", fmt.Errorf("create container request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create helper returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode create helper response: %w", err)
	}
	if strings.TrimSpace(out.ID) == "" {
		return "", fmt.Errorf("create helper returned empty container id")
	}
	return out.ID, nil
}

func (s *Service) startContainer(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("http://localhost/v1.45/containers/%s/start", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create start request: %w", err)
	}
	resp, err := s.docker.Do(req)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *Service) waitContainer(ctx context.Context, id string) (int, string, error) {
	endpoint := fmt.Sprintf("http://localhost/v1.45/containers/%s/wait?condition=not-running", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return 0, "", fmt.Errorf("create wait request: %w", err)
	}
	resp, err := s.docker.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("wait container: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("wait returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		StatusCode int `json:"StatusCode"`
		Error      struct {
			Message string `json:"Message"`
		} `json:"Error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, "", fmt.Errorf("decode wait response: %w", err)
	}
	return out.StatusCode, strings.TrimSpace(out.Error.Message), nil
}

func (s *Service) containerLogs(ctx context.Context, id string, tail int) string {
	endpoint := fmt.Sprintf("http://localhost/v1.45/containers/%s/logs?stdout=1&stderr=1&tail=%d", id, tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	resp, err := s.docker.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func (s *Service) removeContainer(ctx context.Context, id string) {
	endpoint := fmt.Sprintf("http://localhost/v1.45/containers/%s?force=1", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return
	}
	resp, err := s.docker.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func lastLines(text string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, " | ")
	}
	return strings.Join(lines[len(lines)-maxLines:], " | ")
}

func (s *Service) runComposeHelper(ctx context.Context, image, script string, binds []string) error {
	helperName := fmt.Sprintf("wiregate-update-helper-%d", time.Now().UTC().UnixNano())
	createPayload := map[string]any{
		"Image":      image,
		"Entrypoint": []string{"/bin/sh", "-ec"},
		"Cmd":        []string{script},
		"Tty":        true,
		"HostConfig": map[string]any{
			"Binds": binds,
		},
	}
	helperID, err := s.createContainer(ctx, helperName, createPayload)
	if err != nil {
		return err
	}
	defer s.removeContainer(context.Background(), helperID)

	if err := s.startContainer(ctx, helperID); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}

	exitCode, waitMsg, err := s.waitContainer(ctx, helperID)
	if err != nil {
		return fmt.Errorf("wait helper: %w", err)
	}
	if exitCode != 0 {
		logs := lastLines(s.containerLogs(context.Background(), helperID, 200), 10)
		detail := strings.TrimSpace(waitMsg)
		if detail == "" {
			detail = "compose helper exited with non-zero status"
		}
		if logs != "" {
			detail = detail + "; logs: " + logs
		}
		if strings.Contains(strings.ToLower(detail), "manifest unknown") || strings.Contains(strings.ToLower(detail), "pull access denied") {
			detail += "; verify that release images for target version exist and are readable in GHCR"
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}

// restartSelf sends SIGHUP-like restart: stop self, Docker restart policy brings it back with new image.
// For compose setups, we stop the container — `restart: unless-stopped` will NOT restart a manually stopped container.
// Instead we use the Docker API restart endpoint which stops and starts in one call.
func (s *Service) restartSelf(containerID string) error {
	restartURL := fmt.Sprintf("http://localhost/v1.45/containers/%s/restart?t=5", containerID)
	resp, err := s.docker.Post(restartURL, "", nil)
	if err != nil {
		return fmt.Errorf("restart container: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("restart returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *Service) runUpdate(targetVersion string) {
	s.logger.Printf("update: starting update to %s", targetVersion)

	// Step 1: Discover self.
	info, err := s.selfInspect()
	if err != nil {
		s.logger.Printf("update: self-inspect failed: %v", err)
		s.setStatus("failed", fmt.Sprintf("self-inspect failed: %v", err))
		return
	}
	s.logger.Printf("update: container=%s image=%s", shortContainerID(info.ID), info.Image)

	targetBackendImage, err := resolveTargetImage(info.Image, targetVersion)
	if err != nil {
		s.logger.Printf("update: target backend image resolve failed: %v", err)
		s.setStatus("failed", fmt.Sprintf("cannot resolve target backend image from %q and version %q: %v", info.Image, targetVersion, err))
		return
	}

	s.setStatus("pulling", fmt.Sprintf("pulling backend image %s...", targetBackendImage))
	if err := s.pullImage(context.Background(), targetBackendImage); err != nil {
		s.logger.Printf("update: backend pull failed: %v", err)
		s.setStatus("failed", err.Error())
		return
	}
	s.logger.Printf("update: backend image pulled: %s", targetBackendImage)

	composeMeta, hasCompose, err := parseComposeContext(info.Labels)
	if err != nil {
		s.logger.Printf("update: compose context error: %v", err)
		s.setStatus("failed", fmt.Sprintf("compose metadata is incomplete: %v", err))
		return
	}

	if hasCompose {
		frontendImage, err := discoverComposeServiceImage(context.Background(), s.docker, composeMeta.Project, "frontend")
		if err != nil {
			s.logger.Printf("update: frontend image lookup failed: %v", err)
			s.setStatus("failed", fmt.Sprintf("compose frontend discovery failed: %v", err))
			return
		}

		targetFrontendImage := ""
		if frontendImage != "" {
			targetFrontendImage, err = resolveTargetImage(frontendImage, targetVersion)
			if err != nil {
				s.logger.Printf("update: frontend target resolve failed: %v", err)
				s.setStatus("failed", fmt.Sprintf("cannot resolve target frontend image from %q and version %q: %v", frontendImage, targetVersion, err))
				return
			}
			s.setStatus("pulling", fmt.Sprintf("pulling frontend image %s...", targetFrontendImage))
			if err := s.pullImage(context.Background(), targetFrontendImage); err != nil {
				s.logger.Printf("update: frontend pull failed: %v", err)
				s.setStatus("failed", err.Error())
				return
			}
			s.logger.Printf("update: frontend image pulled: %s", targetFrontendImage)
		}

		script := buildComposeUpdateScript(composeMeta, targetBackendImage, targetFrontendImage)
		binds := helperBinds(composeMeta, info.Mounts)
		s.setStatus("restarting", fmt.Sprintf("applying %s with docker compose...", strings.TrimSpace(targetVersion)))
		s.logger.Printf("update: pulling docker:cli helper image")
		if err := s.pullImage(context.Background(), "docker:cli"); err != nil {
			s.logger.Printf("update: docker:cli pull failed: %v", err)
			s.setStatus("failed", fmt.Sprintf("failed to pull helper image: %v", err))
			return
		}
		s.logger.Printf("update: running compose helper for project=%s", composeMeta.Project)
		if err := s.runComposeHelper(context.Background(), "docker:cli", script, binds); err != nil {
			s.logger.Printf("update: compose helper failed: %v", err)
			s.setStatus("failed", fmt.Sprintf("compose update failed: %v", err))
			return
		}
		// If backend got recreated this process usually terminates before reaching this line.
		s.setStatus("idle", "")
		return
	}

	// Non-compose fallback: pull target tag and restart this container only.
	s.setStatus("restarting", fmt.Sprintf("restarting container %s...", info.Name))
	s.logger.Printf("update: compose labels not found, using restart fallback")
	if err := s.restartSelf(info.ID); err != nil {
		s.logger.Printf("update: restart failed: %v", err)
		s.setStatus("failed", fmt.Sprintf("restart failed after pull: %v", err))
		return
	}
	// Should not reach here — process dies during restart.
	s.setStatus("idle", "")
}

func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// registryAuthFromConfig reads ~/.docker/config.json (or /root/.docker/config.json)
// and returns the base64-encoded X-Registry-Auth header for the registry of the given image.
func registryAuthFromConfig(image string) string {
	registry := "https://index.docker.io/v1/"
	if parts := strings.SplitN(image, "/", 2); len(parts) == 2 && strings.Contains(parts[0], ".") {
		registry = parts[0]
	}

	configPaths := []string{
		filepath.Join(os.Getenv("HOME"), ".docker", "config.json"),
		"/app/.docker/config.json",
		"/root/.docker/config.json",
	}

	for _, cfgPath := range configPaths {
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			continue
		}
		var cfg struct {
			Auths map[string]struct {
				Auth string `json:"auth"`
			} `json:"auths"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			continue
		}
		if entry, ok := cfg.Auths[registry]; ok && entry.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				continue
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				continue
			}
			authObj, _ := json.Marshal(map[string]string{
				"username":      parts[0],
				"password":      parts[1],
				"serveraddress": registry,
			})
			return base64.URLEncoding.EncodeToString(authObj)
		}
	}
	return ""
}

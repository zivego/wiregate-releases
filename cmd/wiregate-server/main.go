package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zivego/wiregate/internal/analytics"
	"github.com/zivego/wiregate/internal/api"
	"github.com/zivego/wiregate/internal/apikey"
	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/bootstrap"
	"github.com/zivego/wiregate/internal/capacity"
	"github.com/zivego/wiregate/internal/cluster"
	"github.com/zivego/wiregate/internal/config"
	wiredns "github.com/zivego/wiregate/internal/dns"
	"github.com/zivego/wiregate/internal/enrollment"
	"github.com/zivego/wiregate/internal/events"
	"github.com/zivego/wiregate/internal/ipam"
	wirelogging "github.com/zivego/wiregate/internal/logging"
	wiremetrics "github.com/zivego/wiregate/internal/metrics"
	wirenetwork "github.com/zivego/wiregate/internal/network"
	"github.com/zivego/wiregate/internal/notification"
	"github.com/zivego/wiregate/internal/persistence/accesspolicyrepo"
	"github.com/zivego/wiregate/internal/persistence/agentrepo"
	"github.com/zivego/wiregate/internal/persistence/analyticsrepo"
	"github.com/zivego/wiregate/internal/persistence/auditrepo"
	"github.com/zivego/wiregate/internal/persistence/capacityrepo"
	"github.com/zivego/wiregate/internal/persistence/clusterrepo"
	"github.com/zivego/wiregate/internal/persistence/db"
	"github.com/zivego/wiregate/internal/persistence/dnsconfigrepo"
	"github.com/zivego/wiregate/internal/persistence/enrollmenttokenrepo"
	"github.com/zivego/wiregate/internal/persistence/ipamrepo"
	"github.com/zivego/wiregate/internal/persistence/loggingrepo"
	"github.com/zivego/wiregate/internal/persistence/metricsrepo"
	"github.com/zivego/wiregate/internal/persistence/migrate"
	"github.com/zivego/wiregate/internal/persistence/notificationrepo"
	"github.com/zivego/wiregate/internal/persistence/peerrepo"
	"github.com/zivego/wiregate/internal/persistence/policyassignmentrepo"
	"github.com/zivego/wiregate/internal/persistence/ratelimitrepo"
	"github.com/zivego/wiregate/internal/persistence/runtimesyncrepo"
	"github.com/zivego/wiregate/internal/persistence/securityapprovalrepo"
	"github.com/zivego/wiregate/internal/persistence/securitypolicyrepo"
	"github.com/zivego/wiregate/internal/persistence/serviceaccountrepo"
	"github.com/zivego/wiregate/internal/persistence/sessionrepo"
	"github.com/zivego/wiregate/internal/persistence/userrepo"
	"github.com/zivego/wiregate/internal/policy"
	"github.com/zivego/wiregate/internal/reconcile"
	"github.com/zivego/wiregate/internal/security"
	"github.com/zivego/wiregate/internal/updater"
	"github.com/zivego/wiregate/internal/wgcontrol"
	"github.com/zivego/wiregate/pkg/wgadapter"
	"github.com/zivego/wiregate/pkg/wgadapter/fake"
	"github.com/zivego/wiregate/pkg/wgadapter/kernel"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := config.LoadFromEnv()
	logger := log.New(os.Stdout, "wiregate: ", log.LstdFlags|log.LUTC)

	// Open database.
	var (
		sqlDB *db.Handle
		err   error
	)
	switch strings.ToLower(strings.TrimSpace(cfg.DBDriver)) {
	case "", "sqlite":
		sqlDB, err = db.OpenSQLite(cfg.SQLiteDBPath)
	case "postgres":
		sqlDB, err = db.OpenPostgres(cfg.PostgresDSN)
	default:
		logger.Fatalf("unsupported db driver: %s", cfg.DBDriver)
	}
	if err != nil {
		logger.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()

	// Run migrations.
	ctx := context.Background()
	if err := migrate.ApplyAll(ctx, db.NewExecutor(sqlDB)); err != nil {
		logger.Fatalf("migrate: %v", err)
	}

	// Wire repositories and services.
	users := userrepo.New(sqlDB)
	sessions := sessionrepo.New(sqlDB)
	audits := auditrepo.New(sqlDB)
	agents := agentrepo.New(sqlDB)
	peers := peerrepo.New(sqlDB)
	accessPolicies := accesspolicyrepo.New(sqlDB)
	policyAssignments := policyassignmentrepo.New(sqlDB)
	enrollmentTokens := enrollmenttokenrepo.New(sqlDB)
	runtimeSync := runtimesyncrepo.New(sqlDB)
	securityPolicies := securitypolicyrepo.New(sqlDB)
	securityApprovals := securityapprovalrepo.New(sqlDB)
	analyticsRepo := analyticsrepo.New(sqlDB)
	capacityRepo := capacityrepo.New(sqlDB)
	logExportRepo := loggingrepo.New(sqlDB)
	dnsConfigRepo := dnsconfigrepo.New(sqlDB)
	auditSvc := audit.NewService(audits)
	authSvc := auth.NewService(users, sessions, cfg.SessionTTL, cfg.SessionIdleTimeout)

	// Bootstrap first admin if configured.
	if cfg.BootstrapAdminEmail != "" && cfg.BootstrapAdminPassword != "" {
		if err := bootstrap.MaybeCreateAdmin(ctx, users, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword, logger); err != nil {
			logger.Fatalf("bootstrap: %v", err)
		}
	}

	var wgAdapter wgadapter.Adapter
	switch strings.ToLower(strings.TrimSpace(cfg.WGAdapter)) {
	case "kernel":
		logger.Printf("using kernel WireGuard adapter on interface %s", cfg.WGInterface)
		wgAdapter = kernel.New(cfg.WGInterface)
	default:
		logger.Printf("using fake WireGuard adapter (set WIREGATE_WG_ADAPTER=kernel for real WG)")
		wgAdapter = fake.New()
	}
	wgService := wgcontrol.NewService(wgAdapter)
	policySvc := policy.NewService(accessPolicies, policyAssignments, agents, peers)
	analyticsSvc := analytics.NewService(analyticsRepo, nil)
	loggingSvc := wirelogging.NewService(logExportRepo, logger, wirelogging.Options{})
	capacitySvc := capacity.NewService(capacityRepo, loggingSvc, cfg.SessionIdleTimeout, nil)
	dnsSvc := wiredns.NewService(dnsConfigRepo)
	networkSvc := wirenetwork.NewService(agents, peers, runtimeSync, policySvc)
	securitySvc := security.NewService(securityPolicies, securityApprovals, security.Policy{
		RequiredAdminAMR: strings.TrimSpace(cfg.OIDCRequiredAdminAMR),
		RequiredAdminACR: strings.TrimSpace(cfg.OIDCRequiredAdminACR),
	})
	enrollmentSvc := enrollment.NewService(enrollmentTokens, agents, peers, enrollment.BootstrapConfig{
		ServerEndpoint:  cfg.WGServerEndpoint,
		ServerPublicKey: cfg.WGServerPublicKey,
		ClientCIDR:      cfg.WGClientCIDR,
	}, policySvc, runtimeSync)
	enrollmentSvc.SetDNSProvider(dnsSvc)
	reconcileSvc := reconcile.NewService(peers, agents, wgService, runtimeSync)
	metricsRepo := metricsrepo.New(sqlDB)
	rateLimitRepo := ratelimitrepo.New(sqlDB)
	notifPrefsRepo := notificationrepo.New(sqlDB)
	ipamRepo := ipamrepo.New(sqlDB)
	ipamSvc := ipam.NewService(ipamRepo)
	serviceAccountRepo := serviceaccountrepo.New(sqlDB)
	apiKeySvc := apikey.NewService(serviceAccountRepo)
	clusterRepo := clusterrepo.New(sqlDB)
	clusterSvc := cluster.NewService(clusterRepo, cluster.Config{
		Enabled:           cfg.ClusterEnabled,
		InstanceID:        cfg.ClusterInstanceID,
		LeaseName:         cfg.ClusterLeaseName,
		LeaseTTL:          cfg.ClusterLeaseTTL,
		HeartbeatInterval: cfg.ClusterHeartbeat,
	})
	api.ConfigureExtensions(apiKeySvc, apiKeySvc, clusterSvc)
	api.ConfigureAuditExport(api.AuditExportConfig{
		Directory:  cfg.AuditExportDir,
		S3Region:   cfg.AuditExportS3Region,
		S3Bucket:   cfg.AuditExportS3Bucket,
		S3Prefix:   cfg.AuditExportS3Prefix,
		S3Endpoint: cfg.AuditExportS3Endpoint,
		S3Insecure: cfg.AuditExportS3Insecure,
	})
	if cfg.UpdateEnabled {
		updaterSvc := updater.NewService(updater.Config{
			ManifestURL: cfg.UpdateManifestURL,
		}, logger)
		api.ConfigureUpdater(updaterSvc)
		logger.Printf("server update feature enabled (manifest: %s)", cfg.UpdateManifestURL)
	}

	// Start background metrics collector (every 30s).
	metricsCtx, metricsCancel := context.WithCancel(ctx)
	defer metricsCancel()
	wiremetrics.StartBackgroundCollector(metricsCtx, wiremetrics.BusinessMetricsSource{
		WG:        wgService,
		Reconcile: reconcileSvc,
		Counts:    metricsRepo,
	}, 30*time.Second, logger)

	eventBroker := events.NewBroker()

	handler := api.NewRouter(
		logger,
		wgService,
		auditSvc,
		enrollmentSvc,
		policySvc,
		reconcileSvc,
		authSvc,
		securitySvc,
		analyticsSvc,
		capacitySvc,
		loggingSvc,
		dnsSvc,
		networkSvc,
		users,
		eventBroker,
		rateLimitRepo,
		notifPrefsRepo,
		ipamSvc,
		api.AgentUpdateConfig{
			Version: cfg.AgentUpdateVersion,
			BaseURL: cfg.AgentUpdateBaseURL,
		},
		api.OIDCConfig{
			DisplayName:      cfg.OIDCDisplayName,
			IssuerURL:        cfg.OIDCIssuerURL,
			ClientID:         cfg.OIDCClientID,
			ClientSecret:     cfg.OIDCClientSecret,
			RedirectURL:      cfg.OIDCRedirectURL,
			Scopes:           cfg.OIDCScopes,
			AdminGroups:      cfg.OIDCAdminGroups,
			OperatorGroups:   cfg.OIDCOperatorGroups,
			ReadonlyGroups:   cfg.OIDCReadonlyGroups,
			RequiredAdminAMR: cfg.OIDCRequiredAdminAMR,
			RequiredAdminACR: cfg.OIDCRequiredAdminACR,
			AutoCreateUsers:  cfg.OIDCAutoCreateUsers,
		},
		cfg.CookieInsecure,
		cfg.APIMaxBodyBytes,
		cfg.APIMaxJSONBytes,
	)

	// Start notification service (listens to event broker, sends emails).
	if cfg.NotificationsEnabled && cfg.SMTPHost != "" {
		notifSender := notification.NewSMTPSender(notification.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
			TLS:      cfg.SMTPTLS,
		})
		notifSvc := notification.NewService(
			notification.Config{Enabled: true, Sender: notifSender},
			users, notifPrefsRepo, logger,
		)
		go notifSvc.Start(metricsCtx, eventBroker)
		logger.Printf("notification service enabled (smtp=%s:%d)", cfg.SMTPHost, cfg.SMTPPort)
	}

	// Start rate limit cleanup goroutine (every 2 minutes).
	api.StartRateLimitCleanup(metricsCtx, rateLimitRepo, 2*time.Minute, logger)
	clusterSvc.Start(metricsCtx)

	// Build top-level mux: API routes + /metrics endpoint.
	topMux := http.NewServeMux()
	topMux.Handle("/", handler)
	topMux.Handle("GET /metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         cfg.HTTPAddress,
		Handler:      wiremetrics.Middleware(topMux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s", cfg.HTTPAddress)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
}

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/zivego/wiregate/internal/analytics"
	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/capacity"
	wiredns "github.com/zivego/wiregate/internal/dns"
	"github.com/zivego/wiregate/internal/enrollment"
	"github.com/zivego/wiregate/internal/events"
	"github.com/zivego/wiregate/internal/ipam"
	wirelogging "github.com/zivego/wiregate/internal/logging"
	wirenetwork "github.com/zivego/wiregate/internal/network"
	"github.com/zivego/wiregate/internal/persistence/notificationrepo"
	"github.com/zivego/wiregate/internal/persistence/ratelimitrepo"
	"github.com/zivego/wiregate/internal/persistence/userrepo"
	"github.com/zivego/wiregate/internal/policy"
	"github.com/zivego/wiregate/internal/reconcile"
	"github.com/zivego/wiregate/internal/security"
	"github.com/zivego/wiregate/internal/wgcontrol"
)

// Router holds dependencies for HTTP handlers.
type Router struct {
	logger             *log.Logger
	wgService          *wgcontrol.Service
	auditService       *audit.Service
	enrollmentService  *enrollment.Service
	policyService      *policy.Service
	reconcileService   *reconcile.Service
	authService        *auth.Service
	securityService    *security.Service
	analyticsService   *analytics.Service
	capacityService    *capacity.Service
	loggingService     *wirelogging.Service
	dnsService         *wiredns.Service
	networkService     *wirenetwork.Service
	users              *userrepo.Repo
	eventBroker        *events.Broker
	notificationPrefs  *notificationrepo.Repo
	ipamService        *ipam.Service
	oidc               *oidcAuthenticator
	loginLimiter       rateLimiter
	enrollLimiter      rateLimiter
	sensitiveLimiter   rateLimiter
	apiKeyAuth         apiKeyAuthenticator
	serviceAccounts    serviceAccountManager
	cluster            clusterStatusProvider
	auditExport        AuditExportConfig
	cookieInsecure     bool
	maxJSONBytes       int64
	agentUpdateVersion string
	agentUpdateBaseURL string
}

// AgentUpdateConfig holds the desired agent version and download URL.
type AgentUpdateConfig struct {
	Version string
	BaseURL string
}

func NewRouter(logger *log.Logger, wgService *wgcontrol.Service, auditService *audit.Service, enrollmentService *enrollment.Service, policyService *policy.Service, reconcileService *reconcile.Service, authService *auth.Service, securityService *security.Service, analyticsService *analytics.Service, capacityService *capacity.Service, loggingService *wirelogging.Service, dnsService *wiredns.Service, networkService *wirenetwork.Service, users *userrepo.Repo, eventBroker *events.Broker, rateLimitRepo *ratelimitrepo.Repo, notifPrefs *notificationrepo.Repo, ipamSvc *ipam.Service, agentUpdate AgentUpdateConfig, oidcConfig OIDCConfig, cookieInsecure bool, maxBodyBytes int64, maxJSONBytes int64) http.Handler {
	if maxJSONBytes <= 0 {
		maxJSONBytes = 65536
	}
	oidcAuth, err := newOIDCAuthenticator(oidcConfig)
	if err != nil {
		logger.Printf("oidc disabled due to config error: %v", err)
	}

	// Use DB-backed rate limiters when a repo is available, otherwise in-memory.
	mkLimiter := func(maxN int, window time.Duration) rateLimiter {
		if rateLimitRepo != nil {
			return newDBRateLimiter(rateLimitRepo, maxN, window)
		}
		return newRateLimiter(maxN, window)
	}

	r := &Router{
		logger:             logger,
		wgService:          wgService,
		auditService:       auditService,
		enrollmentService:  enrollmentService,
		policyService:      policyService,
		reconcileService:   reconcileService,
		authService:        authService,
		securityService:    securityService,
		analyticsService:   analyticsService,
		capacityService:    capacityService,
		loggingService:     loggingService,
		dnsService:         dnsService,
		networkService:     networkService,
		users:              users,
		eventBroker:        eventBroker,
		notificationPrefs:  notifPrefs,
		ipamService:        ipamSvc,
		oidc:               oidcAuth,
		loginLimiter:       mkLimiter(5, 60*time.Second),
		enrollLimiter:      mkLimiter(20, 60*time.Second),
		sensitiveLimiter:   mkLimiter(20, 60*time.Second),
		apiKeyAuth:         getAPIKeyAuthenticator(),
		serviceAccounts:    getServiceAccountManager(),
		cluster:            getClusterStatusProvider(),
		auditExport:        getAuditExportConfig(),
		cookieInsecure:     cookieInsecure,
		maxJSONBytes:       maxJSONBytes,
		agentUpdateVersion: agentUpdate.Version,
		agentUpdateBaseURL: agentUpdate.BaseURL,
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/openapi.yaml", r.handleOpenAPISpec)
	mux.HandleFunc("GET /docs", r.handleDocs)

	mux.HandleFunc("GET /api/v1/health/live", r.handleLive)
	mux.HandleFunc("GET /api/v1/health/ready", r.handleReady)
	mux.HandleFunc("GET /api/v1/health/reconcile", r.handleReconcile)
	mux.HandleFunc("GET /api/v1/analytics/dashboard", r.handleGetDashboardAnalytics)
	mux.HandleFunc("GET /api/v1/analytics/audit", r.handleGetAuditAnalytics)
	mux.HandleFunc("GET /api/v1/system/capacity", r.handleGetSystemCapacity)
	mux.HandleFunc("GET /api/v1/system/cluster", r.handleGetClusterStatus)
	mux.HandleFunc("GET /api/v1/system/version", r.handleGetSystemVersion)
	mux.HandleFunc("GET /api/v1/system/update/check", r.handleCheckForUpdate)
	mux.HandleFunc("POST /api/v1/system/update/apply", r.handleApplyUpdate)
	mux.HandleFunc("GET /api/v1/system/update/status", r.handleUpdateStatus)
	mux.HandleFunc("GET /api/v1/auth/providers", r.handleAuthProviders)
	mux.HandleFunc("GET /api/v1/auth/oidc/start", r.handleOIDCStart)
	mux.HandleFunc("GET /api/v1/auth/oidc/callback", r.handleOIDCCallback)
	mux.HandleFunc("GET /api/v1/dns/config", r.handleGetDNSConfig)
	mux.HandleFunc("PATCH /api/v1/dns/config", r.handlePatchDNSConfig)
	mux.HandleFunc("GET /api/v1/network/diagnostics", r.handleGetNetworkDiagnostics)
	mux.HandleFunc("GET /api/v1/logging/sinks", r.handleListLoggingSinks)
	mux.HandleFunc("POST /api/v1/logging/sinks", r.handleCreateLoggingSink)
	mux.HandleFunc("PATCH /api/v1/logging/sinks/", r.handlePatchLoggingSink)
	mux.HandleFunc("DELETE /api/v1/logging/sinks/", r.handleDeleteLoggingSink)
	mux.HandleFunc("GET /api/v1/logging/routes", r.handleGetLoggingRoutes)
	mux.HandleFunc("PATCH /api/v1/logging/routes", r.handlePatchLoggingRoutes)
	mux.HandleFunc("GET /api/v1/logging/status", r.handleGetLoggingStatus)
	mux.HandleFunc("POST /api/v1/logging/test-delivery", r.handleTestLoggingDelivery)
	mux.HandleFunc("GET /api/v1/security/policies", r.handleGetSecurityPolicy)
	mux.HandleFunc("PATCH /api/v1/security/policies", r.handlePatchSecurityPolicy)
	mux.HandleFunc("GET /api/v1/security/approvals", r.handleListSecurityApprovals)
	mux.HandleFunc("POST /api/v1/security/approvals/", r.handleSecurityApprovalPost)

	mux.HandleFunc("POST /api/v1/sessions", r.handleLogin)
	mux.HandleFunc("GET /api/v1/sessions", r.handleListSessions)
	mux.HandleFunc("GET /api/v1/sessions/current", r.handleSessionCurrent)
	mux.HandleFunc("DELETE /api/v1/sessions/current", r.handleLogout)
	mux.HandleFunc("POST /api/v1/sessions/", r.handleSessionPost)

	mux.HandleFunc("GET /api/v1/users", r.handleListUsers)
	mux.HandleFunc("POST /api/v1/users", r.handleCreateUser)
	mux.HandleFunc("PATCH /api/v1/users/me/preferences", r.handleUpdateOwnPreferences)
	mux.HandleFunc("POST /api/v1/users/me/password", r.handleChangeOwnPassword)
	mux.HandleFunc("GET /api/v1/users/me/mfa/totp", r.handleGetOwnTOTPStatus)
	mux.HandleFunc("POST /api/v1/users/me/mfa/totp/setup", r.handleSetupOwnTOTP)
	mux.HandleFunc("POST /api/v1/users/me/mfa/totp/enable", r.handleEnableOwnTOTP)
	mux.HandleFunc("POST /api/v1/users/me/mfa/totp/disable", r.handleDisableOwnTOTP)
	mux.HandleFunc("PATCH /api/v1/users/", r.handlePatchUser)
	mux.HandleFunc("POST /api/v1/users/", r.handleUserPost)
	mux.HandleFunc("DELETE /api/v1/users/", r.handleDeleteUser)
	mux.HandleFunc("GET /api/v1/service-accounts", r.handleListServiceAccounts)
	mux.HandleFunc("POST /api/v1/service-accounts", r.handleCreateServiceAccount)
	mux.HandleFunc("GET /api/v1/service-accounts/", r.handleServiceAccountGet)
	mux.HandleFunc("POST /api/v1/service-accounts/", r.handleServiceAccountPost)

	mux.HandleFunc("GET /api/v1/agents", r.handleListAgents)
	mux.HandleFunc("POST /api/v1/agents/bulk-state", r.handleBulkAgentState)
	mux.HandleFunc("GET /api/v1/agents/update-manifest", r.handleGetUpdateManifest)
	mux.HandleFunc("GET /api/v1/agents/", r.handleGetAgent)
	mux.HandleFunc("PATCH /api/v1/agents/", r.handlePatchAgent)
	mux.HandleFunc("POST /api/v1/agents/", r.handleAgentPost)

	mux.HandleFunc("GET /api/v1/peers", r.handleListPeers)
	mux.HandleFunc("GET /api/v1/peers/", r.handleGetPeer)
	mux.HandleFunc("POST /api/v1/peers/", r.handleReconcilePeer)

	mux.HandleFunc("POST /api/v1/enrollment-tokens", r.handleCreateEnrollmentToken)
	mux.HandleFunc("GET /api/v1/enrollment-tokens", r.handleListEnrollmentTokens)
	mux.HandleFunc("POST /api/v1/enrollment-tokens/", r.handleRevokeEnrollmentToken)
	mux.HandleFunc("POST /api/v1/enrollments", r.handlePerformEnrollment)

	mux.HandleFunc("GET /api/v1/access-policies", r.handleListAccessPolicies)
	mux.HandleFunc("POST /api/v1/access-policies", r.handleCreateAccessPolicy)
	mux.HandleFunc("POST /api/v1/access-policies/simulate", r.handleSimulateAccessPolicy)
	mux.HandleFunc("POST /api/v1/access-policies/bulk-assign", r.handleBulkAssignPolicies)
	mux.HandleFunc("PATCH /api/v1/access-policies/", r.handleUpdateAccessPolicy)
	mux.HandleFunc("POST /api/v1/access-policies/", r.handleAssignAccessPolicy)

	mux.HandleFunc("GET /api/v1/audit-events", r.handleListAuditEvents)
	mux.HandleFunc("POST /api/v1/audit-events/export", r.handleExportAuditEvents)
	mux.HandleFunc("GET /api/v1/events", r.handleSSE)

	mux.HandleFunc("GET /api/v1/notifications/preferences", r.handleGetNotificationPreferences)
	mux.HandleFunc("PATCH /api/v1/notifications/preferences", r.handlePatchNotificationPreferences)

	mux.HandleFunc("GET /api/v1/ipam/pools", r.handleListIPAMPools)
	mux.HandleFunc("POST /api/v1/ipam/pools", r.handleCreateIPAMPool)
	mux.HandleFunc("GET /api/v1/ipam/pools/", r.handleIPAMPoolGet)
	mux.HandleFunc("DELETE /api/v1/ipam/pools/", r.handleDeleteIPAMPool)
	mux.HandleFunc("POST /api/v1/ipam/pools/", r.handleIPAMPoolPost)
	mux.HandleFunc("DELETE /api/v1/ipam/reservations/", r.handleReleaseIPAMReservation)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if maxBodyBytes > 0 && req.Body != nil && req.Body != http.NoBody {
			req.Body = http.MaxBytesReader(w, req.Body, maxBodyBytes)
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, req)
	})
}

func (r *Router) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func (r *Router) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}

func (r *Router) handleReconcile(w http.ResponseWriter, req *http.Request) {
	if err := r.wgService.Ping(req.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded",
			"error":  err.Error(),
		})
		return
	}

	if r.reconcileService == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ready",
			"drift":  "unknown",
		})
		return
	}

	summary, err := r.reconcileService.Summarize(req.Context())
	if err != nil {
		r.logger.Printf("reconcile summary error: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded",
			"drift":  "unknown",
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      summary.Status,
		"drift":       summary.DriftStatus,
		"peer_count":  summary.PeerCount,
		"drift_count": summary.Drifted,
	})
}

func (r *Router) handleNotImplemented(w http.ResponseWriter, req *http.Request) {
	r.logger.Printf("not implemented: %s %s", req.Method, req.URL.Path)
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"error": map[string]any{
			"code":    "not_implemented",
			"message": "endpoint scaffold only",
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

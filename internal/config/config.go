package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddress     = ":8080"
	defaultDBDriver        = "sqlite"
	defaultSQLiteDBPath    = "./data/wiregate.db"
	defaultSessionTTLHours = 8
	defaultSessionIdleMins = 30
	defaultWGClientCIDR    = "10.77.0.0/24"
	defaultAPIMaxBodyBytes = 1048576
	defaultAPIMaxJSONBytes = 65536
	defaultAuditExportDir  = "./data/exports/audit"
)

// Config stores process-level configuration values.
type Config struct {
	HTTPAddress            string
	DBDriver               string
	SQLiteDBPath           string
	PostgresDSN            string
	SessionTTL             time.Duration
	SessionIdleTimeout     time.Duration
	APIMaxBodyBytes        int64
	APIMaxJSONBytes        int64
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
	CookieInsecure         bool
	WGAdapter              string
	WGInterface            string
	WGServerEndpoint       string
	WGServerPublicKey      string
	WGClientCIDR           string
	OIDCDisplayName        string
	OIDCIssuerURL          string
	OIDCClientID           string
	OIDCClientSecret       string
	OIDCRedirectURL        string
	OIDCScopes             []string
	OIDCAdminGroups        []string
	OIDCOperatorGroups     []string
	OIDCReadonlyGroups     []string
	OIDCRequiredAdminAMR   string
	OIDCRequiredAdminACR   string
	OIDCAutoCreateUsers    bool
	SMTPHost               string
	SMTPPort               int
	SMTPUser               string
	SMTPPassword           string
	SMTPFrom               string
	SMTPTLS                bool
	NotificationsEnabled   bool
	AgentUpdateVersion     string
	AgentUpdateBaseURL     string
	ClusterEnabled         bool
	ClusterInstanceID      string
	ClusterLeaseName       string
	ClusterLeaseTTL        time.Duration
	ClusterHeartbeat       time.Duration
	AuditExportDir         string
	AuditExportS3Region    string
	AuditExportS3Bucket    string
	AuditExportS3Prefix    string
	AuditExportS3Endpoint  string
	AuditExportS3Insecure  bool
	UpdateEnabled     bool
	UpdateManifestURL string
}

// LoadFromEnv resolves config with conservative defaults for local development.
func LoadFromEnv() Config {
	ttlHours := defaultSessionTTLHours
	if v := os.Getenv("WIREGATE_SESSION_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttlHours = n
		}
	}
	idleMins := defaultSessionIdleMins
	if v := os.Getenv("WIREGATE_SESSION_IDLE_TIMEOUT_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			idleMins = n
		}
	}
	hostname, _ := os.Hostname()
	if strings.TrimSpace(hostname) == "" {
		hostname = "wiregate-node"
	}

	return Config{
		HTTPAddress:            envOrDefault("WIREGATE_HTTP_ADDRESS", defaultHTTPAddress),
		DBDriver:               envOrDefault("WIREGATE_DB_DRIVER", defaultDBDriver),
		SQLiteDBPath:           envOrDefault("WIREGATE_SQLITE_PATH", defaultSQLiteDBPath),
		PostgresDSN:            strings.TrimSpace(envOrDefault("WIREGATE_POSTGRES_DSN", os.Getenv("WIREGATE_DATABASE_URL"))),
		SessionTTL:             time.Duration(ttlHours) * time.Hour,
		SessionIdleTimeout:     time.Duration(idleMins) * time.Minute,
		APIMaxBodyBytes:        envInt64OrDefault("WIREGATE_API_MAX_BODY_BYTES", defaultAPIMaxBodyBytes),
		APIMaxJSONBytes:        envInt64OrDefault("WIREGATE_API_MAX_JSON_BYTES", defaultAPIMaxJSONBytes),
		BootstrapAdminEmail:    os.Getenv("WIREGATE_BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: readSecretValue("WIREGATE_BOOTSTRAP_ADMIN_PASSWORD", "WIREGATE_BOOTSTRAP_ADMIN_PASSWORD_FILE"),
		CookieInsecure:         os.Getenv("WIREGATE_COOKIE_INSECURE") == "true",
		WGAdapter:              envOrDefault("WIREGATE_WG_ADAPTER", "fake"),
		WGInterface:            envOrDefault("WIREGATE_WG_INTERFACE", "wg0"),
		WGServerEndpoint:       os.Getenv("WIREGATE_WG_SERVER_ENDPOINT"),
		WGServerPublicKey:      os.Getenv("WIREGATE_WG_SERVER_PUBLIC_KEY"),
		WGClientCIDR:           envOrDefault("WIREGATE_WG_CLIENT_CIDR", defaultWGClientCIDR),
		OIDCDisplayName:        envOrDefault("WIREGATE_OIDC_DISPLAY_NAME", "Single Sign-On"),
		OIDCIssuerURL:          strings.TrimSpace(os.Getenv("WIREGATE_OIDC_ISSUER_URL")),
		OIDCClientID:           strings.TrimSpace(os.Getenv("WIREGATE_OIDC_CLIENT_ID")),
		OIDCClientSecret:       readSecretValue("WIREGATE_OIDC_CLIENT_SECRET", "WIREGATE_OIDC_CLIENT_SECRET_FILE"),
		OIDCRedirectURL:        strings.TrimSpace(os.Getenv("WIREGATE_OIDC_REDIRECT_URL")),
		OIDCScopes:             csvOrDefault("WIREGATE_OIDC_SCOPES", []string{"openid", "profile", "email", "groups"}),
		OIDCAdminGroups:        csvOrDefault("WIREGATE_OIDC_ADMIN_GROUPS", []string{"wiregate-admin", "admin"}),
		OIDCOperatorGroups:     csvOrDefault("WIREGATE_OIDC_OPERATOR_GROUPS", []string{"wiregate-operator", "operator"}),
		OIDCReadonlyGroups:     csvOrDefault("WIREGATE_OIDC_READONLY_GROUPS", []string{"wiregate-readonly", "readonly"}),
		OIDCRequiredAdminAMR:   strings.TrimSpace(os.Getenv("WIREGATE_OIDC_REQUIRED_ADMIN_AMR")),
		OIDCRequiredAdminACR:   strings.TrimSpace(os.Getenv("WIREGATE_OIDC_REQUIRED_ADMIN_ACR")),
		OIDCAutoCreateUsers:    envBoolOrDefault("WIREGATE_OIDC_AUTO_CREATE_USERS", true),
		SMTPHost:               strings.TrimSpace(os.Getenv("WIREGATE_SMTP_HOST")),
		SMTPPort:               int(envInt64OrDefault("WIREGATE_SMTP_PORT", 587)),
		SMTPUser:               strings.TrimSpace(os.Getenv("WIREGATE_SMTP_USER")),
		SMTPPassword:           readSecretValue("WIREGATE_SMTP_PASSWORD", "WIREGATE_SMTP_PASSWORD_FILE"),
		SMTPFrom:               envOrDefault("WIREGATE_SMTP_FROM", "wiregate@localhost"),
		SMTPTLS:                envBoolOrDefault("WIREGATE_SMTP_TLS", true),
		NotificationsEnabled:   envBoolOrDefault("WIREGATE_NOTIFICATIONS_ENABLED", false),
		AgentUpdateVersion:     strings.TrimSpace(os.Getenv("WIREGATE_AGENT_UPDATE_VERSION")),
		AgentUpdateBaseURL:     strings.TrimSpace(os.Getenv("WIREGATE_AGENT_UPDATE_URL")),
		ClusterEnabled:         envBoolOrDefault("WIREGATE_CLUSTER_ENABLED", false),
		ClusterInstanceID:      envOrDefault("WIREGATE_CLUSTER_INSTANCE_ID", hostname),
		ClusterLeaseName:       envOrDefault("WIREGATE_CLUSTER_LEASE_NAME", "wiregate-main"),
		ClusterLeaseTTL:        time.Duration(envInt64OrDefault("WIREGATE_CLUSTER_LEASE_TTL_SECONDS", 15)) * time.Second,
		ClusterHeartbeat:       time.Duration(envInt64OrDefault("WIREGATE_CLUSTER_HEARTBEAT_SECONDS", 5)) * time.Second,
		AuditExportDir:         envOrDefault("WIREGATE_AUDIT_EXPORT_DIR", defaultAuditExportDir),
		AuditExportS3Region:    strings.TrimSpace(os.Getenv("WIREGATE_AUDIT_EXPORT_S3_REGION")),
		AuditExportS3Bucket:    strings.TrimSpace(os.Getenv("WIREGATE_AUDIT_EXPORT_S3_BUCKET")),
		AuditExportS3Prefix:    strings.TrimSpace(os.Getenv("WIREGATE_AUDIT_EXPORT_S3_PREFIX")),
		AuditExportS3Endpoint:  strings.TrimSpace(os.Getenv("WIREGATE_AUDIT_EXPORT_S3_ENDPOINT")),
		AuditExportS3Insecure:  envBoolOrDefault("WIREGATE_AUDIT_EXPORT_S3_INSECURE", false),
		UpdateEnabled:     envBoolOrDefault("WIREGATE_UPDATE_ENABLED", false),
		UpdateManifestURL: strings.TrimSpace(os.Getenv("WIREGATE_UPDATE_MANIFEST_URL")),
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func readSecretValue(valueKey, fileKey string) string {
	if value := os.Getenv(valueKey); value != "" {
		return value
	}

	path := os.Getenv(fileKey)
	if path == "" {
		return ""
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func envInt64OrDefault(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func csvOrDefault(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

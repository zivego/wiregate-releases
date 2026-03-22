package api

import (
	"context"
	"sync"
	"time"

	"github.com/zivego/wiregate/internal/apikey"
	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/cluster"
	"github.com/zivego/wiregate/internal/updater"
)

type apiKeyAuthenticator interface {
	AuthenticateAPIKey(ctx context.Context, rawToken string) (auth.Claims, bool, error)
}

type serviceAccountManager interface {
	CreateServiceAccount(ctx context.Context, name, role string) (apikey.ServiceAccount, error)
	ListServiceAccounts(ctx context.Context) ([]apikey.ServiceAccount, error)
	CreateAPIKey(ctx context.Context, accountID, name string, ttl time.Duration) (apikey.CreatedKey, error)
	ListAPIKeys(ctx context.Context, accountID string) ([]apikey.APIKey, error)
	RevokeAPIKey(ctx context.Context, accountID, keyID string) error
}

type clusterStatusProvider interface {
	Status(ctx context.Context) (cluster.Status, error)
}

type AuditExportConfig struct {
	Directory  string
	S3Region   string
	S3Bucket   string
	S3Prefix   string
	S3Endpoint string
	S3Insecure bool
}

var (
	extMu              sync.RWMutex
	extAPIKeyAuth      apiKeyAuthenticator
	extServiceAccounts serviceAccountManager
	extClusterStatus   clusterStatusProvider
	extAuditExport     AuditExportConfig
	extUpdater         *updater.Service
)

func ConfigureExtensions(apiKeyAuth apiKeyAuthenticator, serviceAccounts serviceAccountManager, clusterStatus clusterStatusProvider) {
	extMu.Lock()
	defer extMu.Unlock()
	extAPIKeyAuth = apiKeyAuth
	extServiceAccounts = serviceAccounts
	extClusterStatus = clusterStatus
}

func getAPIKeyAuthenticator() apiKeyAuthenticator {
	extMu.RLock()
	defer extMu.RUnlock()
	return extAPIKeyAuth
}

func getServiceAccountManager() serviceAccountManager {
	extMu.RLock()
	defer extMu.RUnlock()
	return extServiceAccounts
}

func getClusterStatusProvider() clusterStatusProvider {
	extMu.RLock()
	defer extMu.RUnlock()
	return extClusterStatus
}

func ConfigureAuditExport(config AuditExportConfig) {
	extMu.Lock()
	defer extMu.Unlock()
	extAuditExport = config
}

func getAuditExportConfig() AuditExportConfig {
	extMu.RLock()
	defer extMu.RUnlock()
	return extAuditExport
}

func ConfigureUpdater(u *updater.Service) {
	extMu.Lock()
	defer extMu.Unlock()
	extUpdater = u
}

func getUpdater() *updater.Service {
	extMu.RLock()
	defer extMu.RUnlock()
	return extUpdater
}

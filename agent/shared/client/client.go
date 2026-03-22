package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/zivego/wiregate/pkg/wgconfig"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type State struct {
	ServerURL                string `json:"server_url"`
	AgentID                  string `json:"agent_id"`
	AgentToken               string `json:"agent_token"`
	Hostname                 string `json:"hostname"`
	PublicKey                string `json:"public_key"`
	Platform                 string `json:"platform"`
	PeerID                   string `json:"peer_id"`
	PrivateKeyPath           string `json:"private_key_path,omitempty"`
	PendingPrivateKeyPath    string `json:"pending_private_key_path,omitempty"`
	PendingPublicKey         string `json:"pending_public_key,omitempty"`
	ConfigPath               string `json:"config_path,omitempty"`
	DesiredConfigFingerprint string `json:"desired_config_fingerprint,omitempty"`
	LastRenderedConfigHash   string `json:"last_rendered_config_hash,omitempty"`
	LastApplyStatus          string `json:"last_apply_status,omitempty"`
	LastApplyError           string `json:"last_apply_error,omitempty"`
	LastAppliedAt            string `json:"last_applied_at,omitempty"`
}

type EnrollResponse struct {
	Agent           enrolledAgent       `json:"agent"`
	AgentAuthToken  string              `json:"agent_auth_token"`
	WireGuardConfig *WireGuardConfigRef `json:"wireguard_config"`
}

type enrolledAgent struct {
	ID       string          `json:"id"`
	Hostname string          `json:"hostname"`
	Platform string          `json:"platform"`
	Peer     enrolledPeerRef `json:"peer"`
}

type enrolledPeerRef struct {
	ID string `json:"id"`
}

type CheckInResponse struct {
	Agent               CheckedInAgent      `json:"agent"`
	ReconfigureRequired bool                `json:"reconfigure_required"`
	DesiredState        string              `json:"desired_state"`
	RotationRequired    bool                `json:"rotation_required"`
	WireGuardConfig     *WireGuardConfigRef `json:"wireguard_config"`
	Version             string              `json:"version"`
	CheckedInAt         string              `json:"checked_in_at"`
	Update              *UpdateDirective    `json:"update,omitempty"`
}

// UpdateDirective tells the agent to self-update.
type UpdateDirective struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type CheckedInAgent struct {
	ID         string         `json:"id"`
	Hostname   string         `json:"hostname"`
	Platform   string         `json:"platform"`
	Status     string         `json:"status"`
	LastSeenAt string         `json:"last_seen_at"`
	Peer       *CheckedInPeer `json:"peer"`
}

type CheckedInPeer struct {
	ID         string   `json:"id"`
	PublicKey  string   `json:"public_key"`
	AllowedIPs []string `json:"allowed_ips"`
	Status     string   `json:"status"`
}

type WireGuardConfigRef struct {
	InterfaceAddress string   `json:"interface_address"`
	ServerEndpoint   string   `json:"server_endpoint"`
	ServerPublicKey  string   `json:"server_public_key"`
	AllowedIPs       []string `json:"allowed_ips"`
	DNSServers       []string `json:"dns_servers,omitempty"`
	DNSSearchDomains []string `json:"dns_search_domains,omitempty"`
}

type LocalStateReport struct {
	ReportedConfigFingerprint string `json:"reported_config_fingerprint,omitempty"`
	LastApplyStatus           string `json:"last_apply_status,omitempty"`
	LastApplyError            string `json:"last_apply_error,omitempty"`
	LastAppliedAt             string `json:"last_applied_at,omitempty"`
}

func Enroll(ctx context.Context, client HTTPClient, serverURL, token, hostname, platform, publicKey string) (State, *WireGuardConfigRef, error) {
	body, _ := json.Marshal(map[string]any{
		"token":      token,
		"hostname":   hostname,
		"platform":   platform,
		"public_key": publicKey,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/api/v1/enrollments", bytes.NewReader(body))
	if err != nil {
		return State{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return State{}, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return State{}, nil, responseError(resp)
	}

	var payload EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return State{}, nil, err
	}

	return State{
		ServerURL:  strings.TrimRight(serverURL, "/"),
		AgentID:    payload.Agent.ID,
		AgentToken: payload.AgentAuthToken,
		Hostname:   payload.Agent.Hostname,
		PublicKey:  publicKey,
		Platform:   payload.Agent.Platform,
		PeerID:     payload.Agent.Peer.ID,
	}, payload.WireGuardConfig, nil
}

func CheckIn(ctx context.Context, client HTTPClient, state State, version string) (CheckInResponse, error) {
	body, _ := json.Marshal(map[string]any{
		"version":             version,
		"local_state":         state.LocalStateReport(),
		"rotation_public_key": strings.TrimSpace(state.PendingPublicKey),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, state.ServerURL+"/api/v1/agents/"+state.AgentID+"/check-in", bytes.NewReader(body))
	if err != nil {
		return CheckInResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+state.AgentToken)

	resp, err := client.Do(req)
	if err != nil {
		return CheckInResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CheckInResponse{}, responseError(resp)
	}

	var payload CheckInResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CheckInResponse{}, err
	}
	return payload, nil
}

func SaveState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func LoadState(path string) (State, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func EnsurePrivateKey(path string) (privateKey string, publicKey string, err error) {
	if body, readErr := os.ReadFile(path); readErr == nil {
		key, parseErr := wgtypes.ParseKey(strings.TrimSpace(string(body)))
		if parseErr != nil {
			return "", "", parseErr
		}
		return key.String(), key.PublicKey().String(), nil
	} else if !os.IsNotExist(readErr) {
		return "", "", readErr
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", "", err
	}
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, []byte(key.String()+"\n"), 0o600); err != nil {
		return "", "", err
	}
	return key.String(), key.PublicKey().String(), nil
}

func LoadPrivateKey(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	key, err := wgtypes.ParseKey(strings.TrimSpace(string(body)))
	if err != nil {
		return "", err
	}
	return key.String(), nil
}

func DefaultPendingPrivateKeyPath(primaryPath string) string {
	primaryPath = strings.TrimSpace(primaryPath)
	if primaryPath == "" {
		return ""
	}
	return primaryPath + ".pending"
}

func PromotePendingPrivateKey(state *State) error {
	if state == nil || strings.TrimSpace(state.PendingPrivateKeyPath) == "" || strings.TrimSpace(state.PrivateKeyPath) == "" {
		return nil
	}
	body, err := os.ReadFile(state.PendingPrivateKeyPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(state.PrivateKeyPath, body, 0o600); err != nil {
		return err
	}
	if err := os.Remove(state.PendingPrivateKeyPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	state.PublicKey = strings.TrimSpace(state.PendingPublicKey)
	state.PendingPrivateKeyPath = ""
	state.PendingPublicKey = ""
	return nil
}

func ClearPendingPrivateKey(state *State) error {
	if state == nil {
		return nil
	}
	if strings.TrimSpace(state.PendingPrivateKeyPath) != "" {
		if err := os.Remove(state.PendingPrivateKeyPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	state.PendingPrivateKeyPath = ""
	state.PendingPublicKey = ""
	return nil
}

func RenderConfig(privateKey string, cfg WireGuardConfigRef) (string, error) {
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" || cfg.InterfaceAddress == "" || cfg.ServerEndpoint == "" || cfg.ServerPublicKey == "" || len(cfg.AllowedIPs) == 0 {
		return "", fmt.Errorf("incomplete wireguard config payload")
	}
	interfaceSection := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s\n", privateKey, cfg.InterfaceAddress)
	if dnsLine := renderDNSLine(cfg); dnsLine != "" {
		interfaceSection += "DNS = " + dnsLine + "\n"
	}
	return fmt.Sprintf("%s\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = %s\nPersistentKeepalive = 25\n",
		interfaceSection,
		cfg.ServerPublicKey,
		cfg.ServerEndpoint,
		strings.Join(cfg.AllowedIPs, ", "),
	), nil
}

func WriteConfig(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o600)
}

func (s State) LocalStateReport() LocalStateReport {
	return LocalStateReport{
		ReportedConfigFingerprint: s.DesiredConfigFingerprint,
		LastApplyStatus:           s.LastApplyStatus,
		LastApplyError:            s.LastApplyError,
		LastAppliedAt:             s.LastAppliedAt,
	}
}

func DesiredConfigFingerprint(cfg *WireGuardConfigRef) string {
	if cfg == nil {
		return ""
	}
	return wgconfig.Fingerprint(cfg.InterfaceAddress, cfg.ServerEndpoint, cfg.ServerPublicKey, cfg.AllowedIPs, cfg.DNSServers, cfg.DNSSearchDomains)
}

func renderDNSLine(cfg WireGuardConfigRef) string {
	entries := make([]string, 0, len(cfg.DNSServers)+len(cfg.DNSSearchDomains))
	entries = append(entries, cfg.DNSServers...)
	entries = append(entries, cfg.DNSSearchDomains...)
	return strings.Join(entries, ", ")
}

func responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if len(body) == 0 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return fmt.Errorf("request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

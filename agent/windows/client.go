package windowsagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	agentclient "github.com/zivego/wiregate/agent/shared/client"
)

const (
	ApplyStatusApplied     = "applied"
	ApplyStatusStaged      = "staged"
	ApplyStatusApplyFailed = "apply_failed"
	ApplyStatusDrifted     = "drifted"
	ApplyStatusDisabled    = "disabled"
)

type HTTPClient = agentclient.HTTPClient
type State = agentclient.State
type EnrollResponse = agentclient.EnrollResponse
type CheckInResponse = agentclient.CheckInResponse
type CheckedInAgent = agentclient.CheckedInAgent
type CheckedInPeer = agentclient.CheckedInPeer
type WireGuardConfigRef = agentclient.WireGuardConfigRef
type LocalStateReport = agentclient.LocalStateReport

func Enroll(ctx context.Context, client HTTPClient, serverURL, token, hostname, publicKey string) (State, *WireGuardConfigRef, error) {
	return agentclient.Enroll(ctx, client, serverURL, token, hostname, "windows", publicKey)
}

func CheckIn(ctx context.Context, client HTTPClient, state State, version string) (CheckInResponse, error) {
	return agentclient.CheckIn(ctx, client, state, version)
}

func SaveState(path string, state State) error {
	return agentclient.SaveState(path, state)
}

func LoadState(path string) (State, error) {
	return agentclient.LoadState(path)
}

func EnsurePrivateKey(path string) (privateKey string, publicKey string, err error) {
	return agentclient.EnsurePrivateKey(path)
}

func LoadPrivateKey(path string) (string, error) {
	return agentclient.LoadPrivateKey(path)
}

func DefaultPendingPrivateKeyPath(primaryPath string) string {
	return agentclient.DefaultPendingPrivateKeyPath(primaryPath)
}

func PromotePendingPrivateKey(state *State) error {
	return agentclient.PromotePendingPrivateKey(state)
}

func ClearPendingPrivateKey(state *State) error {
	return agentclient.ClearPendingPrivateKey(state)
}

func RenderConfig(privateKey string, cfg WireGuardConfigRef) (string, error) {
	return agentclient.RenderConfig(privateKey, cfg)
}

func WriteConfig(path, body string) error {
	return agentclient.WriteConfig(path, body)
}

func DesiredConfigFingerprint(cfg *WireGuardConfigRef) string {
	return agentclient.DesiredConfigFingerprint(cfg)
}

func StageConfig(state *State, privateKey string, cfg *WireGuardConfigRef) error {
	return WriteAndApplyConfig(context.Background(), state, privateKey, cfg, false, nil)
}

func WriteAndApplyConfig(ctx context.Context, state *State, privateKey string, cfg *WireGuardConfigRef, apply bool, runner CommandRunner) error {
	if state == nil || cfg == nil {
		return nil
	}

	body, err := RenderConfig(privateKey, *cfg)
	if err != nil {
		state.DesiredConfigFingerprint = DesiredConfigFingerprint(cfg)
		state.LastApplyStatus = ApplyStatusApplyFailed
		state.LastApplyError = truncateApplyError(err)
		return err
	}
	if err := WriteConfig(state.ConfigPath, body); err != nil {
		state.DesiredConfigFingerprint = DesiredConfigFingerprint(cfg)
		state.LastApplyStatus = ApplyStatusApplyFailed
		state.LastApplyError = truncateApplyError(err)
		return err
	}

	state.DesiredConfigFingerprint = DesiredConfigFingerprint(cfg)
	state.LastRenderedConfigHash = renderedConfigHash(body)
	state.LastApplyError = ""
	state.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	if !apply {
		state.LastApplyStatus = ApplyStatusStaged
		return nil
	}

	if err := ApplyConfig(ctx, runner, state.ConfigPath, ApplyOptions{IgnoreUninstallError: true}); err != nil {
		state.LastApplyStatus = ApplyStatusApplyFailed
		state.LastApplyError = truncateApplyError(err)
		return err
	}
	state.LastApplyStatus = ApplyStatusApplied
	return nil
}

func renderedConfigHash(body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
	return hex.EncodeToString(sum[:])
}

func truncateApplyError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 256 {
		return message[:256]
	}
	return message
}

func interfaceNameFromConfigPath(configPath string) (string, error) {
	if err := validateConfigPath(configPath); err != nil {
		return "", err
	}
	return strings.TrimSuffix(filepathBase(configPath), ".conf"), nil
}

func tunnelServiceName(configPath string) (string, error) {
	ifaceName, err := interfaceNameFromConfigPath(configPath)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("WireGuardTunnel$%s", ifaceName), nil
}

package linuxagent

import (
	"context"

	agentclient "github.com/zivego/wiregate/agent/shared/client"
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
	return agentclient.Enroll(ctx, client, serverURL, token, hostname, "linux", publicKey)
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

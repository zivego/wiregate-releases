package linuxagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	ApplyStatusApplied     = "applied"
	ApplyStatusStaged      = "staged"
	ApplyStatusApplyFailed = "apply_failed"
	ApplyStatusDrifted     = "drifted"
	ApplyStatusDisabled    = "disabled"
)

func RefreshLocalRuntimeState(ctx context.Context, state *State, runner OutputRunner) {
	if state == nil || state.ConfigPath == "" || state.LastRenderedConfigHash == "" {
		return
	}
	if state.LastApplyStatus == ApplyStatusDisabled {
		return
	}

	body, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		state.LastApplyStatus = ApplyStatusDrifted
		state.LastApplyError = fmt.Sprintf("config file unavailable: %v", err)
		return
	}
	if renderedConfigHash(string(body)) == state.LastRenderedConfigHash {
		refreshRuntimeInterfaceState(ctx, state, runner)
		return
	}
	state.LastApplyStatus = ApplyStatusDrifted
	state.LastApplyError = "local config drift detected"
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

	now := time.Now().UTC()
	state.DesiredConfigFingerprint = DesiredConfigFingerprint(cfg)
	state.LastRenderedConfigHash = renderedConfigHash(body)
	state.LastApplyError = ""
	state.LastAppliedAt = now.Format(time.RFC3339)
	if !apply {
		state.LastApplyStatus = ApplyStatusStaged
		return nil
	}

	if err := ApplyConfig(ctx, runner, state.ConfigPath, ApplyOptions{IgnoreDownError: true}); err != nil {
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

func refreshRuntimeInterfaceState(ctx context.Context, state *State, runner OutputRunner) {
	if state == nil || state.DesiredConfigFingerprint == "" || state.LastApplyStatus == ApplyStatusStaged {
		return
	}

	observation, err := InspectRuntime(ctx, runner, state.ConfigPath)
	if err != nil {
		state.LastApplyStatus = ApplyStatusDrifted
		state.LastApplyError = truncateApplyError(fmt.Errorf("runtime inspect failed: %w", err))
		return
	}
	if !observation.InterfacePresent {
		state.LastApplyStatus = ApplyStatusDrifted
		state.LastApplyError = fmt.Sprintf("wireguard interface %s is not present", observation.InterfaceName)
		return
	}
	if observation.ConfigFingerprint == "" || observation.ConfigFingerprint != state.DesiredConfigFingerprint {
		state.LastApplyStatus = ApplyStatusDrifted
		state.LastApplyError = "runtime config fingerprint mismatch"
		return
	}
	state.LastApplyStatus = ApplyStatusApplied
	state.LastApplyError = ""
}

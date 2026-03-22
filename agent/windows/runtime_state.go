package windowsagent

import (
	"context"
	"fmt"
	"os"
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

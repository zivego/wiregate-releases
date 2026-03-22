package linuxagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/zivego/wiregate/agent/shared/selfupdate"
)

const desiredStateDisabled = "disabled"

type SyncOptions struct {
	Version           string
	Apply             bool
	Runner            CommandRunner
	OutputRunner      OutputRunner
	FailureBackoff    time.Duration
	MaxFailureBackoff time.Duration
	OnLoopError       func(err error, consecutiveFailures int, nextRetryIn time.Duration)
	OnUpdateApplied   func(result selfupdate.Result)
}

func SyncOnce(ctx context.Context, client HTTPClient, statePath string, opts SyncOptions) (CheckInResponse, error) {
	state, err := LoadState(statePath)
	if err != nil {
		return CheckInResponse{}, err
	}
	RefreshLocalRuntimeState(ctx, &state, opts.OutputRunner)

	resp, err := CheckIn(ctx, client, state, opts.Version)
	if err != nil {
		if saveErr := SaveState(statePath, state); saveErr != nil {
			return CheckInResponse{}, saveErr
		}
		return CheckInResponse{}, err
	}
	if resp.DesiredState == desiredStateDisabled {
		if err := disableLocalConfig(ctx, &state, opts.Runner); err != nil {
			if saveErr := SaveState(statePath, state); saveErr != nil {
				return CheckInResponse{}, saveErr
			}
			return CheckInResponse{}, err
		}
		if err := SaveState(statePath, state); err != nil {
			return CheckInResponse{}, err
		}
		return resp, nil
	}
	if resp.RotationRequired {
		if err := ensurePendingRotationKey(&state); err != nil {
			if saveErr := SaveState(statePath, state); saveErr != nil {
				return CheckInResponse{}, saveErr
			}
			return CheckInResponse{}, err
		}
		if err := SaveState(statePath, state); err != nil {
			return CheckInResponse{}, err
		}
		return resp, nil
	}
	if resp.ReconfigureRequired {
		privateKey, rotated, err := resolvePrivateKeyForApply(&state, resp)
		if err != nil {
			return CheckInResponse{}, err
		}
		if err := WriteAndApplyConfig(ctx, &state, privateKey, resp.WireGuardConfig, opts.Apply, opts.Runner); err != nil {
			if saveErr := SaveState(statePath, state); saveErr != nil {
				return CheckInResponse{}, saveErr
			}
			return CheckInResponse{}, err
		}
		if rotated {
			if err := PromotePendingPrivateKey(&state); err != nil {
				if saveErr := SaveState(statePath, state); saveErr != nil {
					return CheckInResponse{}, saveErr
				}
				return CheckInResponse{}, err
			}
		}
	}
	if err := SaveState(statePath, state); err != nil {
		return CheckInResponse{}, err
	}

	// Self-update: if server says a newer version is available, download and replace.
	if resp.Update != nil && resp.Update.Version != "" && resp.Update.Version != opts.Version {
		updateResult, err := selfupdate.Apply(ctx, &http.Client{Timeout: 120 * time.Second}, selfupdate.Directive{
			Version: resp.Update.Version,
			URL:     resp.Update.URL,
		})
		if err == nil && updateResult.Updated && opts.OnUpdateApplied != nil {
			opts.OnUpdateApplied(updateResult)
		}
	}

	return resp, nil
}

func disableLocalConfig(ctx context.Context, state *State, runner CommandRunner) error {
	if state == nil {
		return nil
	}
	state.LastApplyError = ""
	state.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	if state.ConfigPath == "" {
		state.LastApplyStatus = ApplyStatusDisabled
		return nil
	}
	if err := DisableConfig(ctx, runner, state.ConfigPath); err != nil {
		state.LastApplyStatus = ApplyStatusApplyFailed
		state.LastApplyError = truncateApplyError(err)
		return err
	}
	state.LastApplyStatus = ApplyStatusDisabled
	return nil
}

func ensurePendingRotationKey(state *State) error {
	if state == nil {
		return nil
	}
	if state.PendingPublicKey != "" {
		return nil
	}
	pendingPath := state.PendingPrivateKeyPath
	if pendingPath == "" {
		pendingPath = DefaultPendingPrivateKeyPath(state.PrivateKeyPath)
	}
	if pendingPath == "" {
		return fmt.Errorf("pending private key path is required")
	}
	_, publicKey, err := EnsurePrivateKey(pendingPath)
	if err != nil {
		return err
	}
	state.PendingPrivateKeyPath = pendingPath
	state.PendingPublicKey = publicKey
	return nil
}

func resolvePrivateKeyForApply(state *State, resp CheckInResponse) (string, bool, error) {
	if state == nil {
		return "", false, fmt.Errorf("agent state is required")
	}
	if state.PendingPublicKey != "" && resp.Agent.Peer != nil && resp.Agent.Peer.PublicKey == state.PendingPublicKey {
		privateKey, err := LoadPrivateKey(state.PendingPrivateKeyPath)
		return privateKey, true, err
	}
	privateKey, err := LoadPrivateKey(state.PrivateKeyPath)
	return privateKey, false, err
}

func RunLoop(ctx context.Context, client HTTPClient, statePath string, interval time.Duration, opts SyncOptions) error {
	if interval <= 0 {
		return errors.New("interval must be positive")
	}
	failureBackoff, maxFailureBackoff := resolveBackoffDurations(opts.FailureBackoff, opts.MaxFailureBackoff)
	consecutiveFailures := 0
	nextDelay := time.Duration(0)

	for {
		if nextDelay > 0 {
			timer := time.NewTimer(nextDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}

		if _, err := SyncOnce(ctx, client, statePath, opts); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			consecutiveFailures++
			nextDelay = computeBackoffDelay(consecutiveFailures, failureBackoff, maxFailureBackoff)
			if opts.OnLoopError != nil {
				opts.OnLoopError(err, consecutiveFailures, nextDelay)
			}
			continue
		}

		consecutiveFailures = 0
		nextDelay = interval
	}
}

func resolveBackoffDurations(failureBackoff, maxFailureBackoff time.Duration) (time.Duration, time.Duration) {
	if failureBackoff <= 0 {
		failureBackoff = 5 * time.Second
	}
	if maxFailureBackoff <= 0 {
		maxFailureBackoff = 1 * time.Minute
	}
	if maxFailureBackoff < failureBackoff {
		maxFailureBackoff = failureBackoff
	}
	return failureBackoff, maxFailureBackoff
}

func computeBackoffDelay(consecutiveFailures int, failureBackoff, maxFailureBackoff time.Duration) time.Duration {
	if consecutiveFailures <= 1 {
		return failureBackoff
	}

	delay := failureBackoff
	for attempt := 1; attempt < consecutiveFailures; attempt++ {
		if delay >= maxFailureBackoff {
			return maxFailureBackoff
		}
		delay *= 2
		if delay >= maxFailureBackoff {
			return maxFailureBackoff
		}
	}
	return delay
}

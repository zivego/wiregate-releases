package windowsagent

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type ServiceUnitOptions struct {
	ServiceName       string
	BinaryPath        string
	StateFile         string
	Version           string
	Interval          time.Duration
	FailureBackoff    time.Duration
	MaxFailureBackoff time.Duration
	Apply             bool
}

type ServiceInstallOptions struct {
	Unit   ServiceUnitOptions
	Runner CommandRunner
}

func RenderServiceCommandLine(opts ServiceUnitOptions) (string, error) {
	if err := validateServiceName(opts.ServiceName); err != nil {
		return "", fmt.Errorf("service name: %w", err)
	}
	if err := validateAbsolutePath(opts.BinaryPath); err != nil {
		return "", fmt.Errorf("binary path: %w", err)
	}
	if err := validateAbsolutePath(opts.StateFile); err != nil {
		return "", fmt.Errorf("state file: %w", err)
	}
	if opts.Interval <= 0 {
		return "", fmt.Errorf("interval must be positive")
	}
	if strings.TrimSpace(opts.Version) == "" {
		return "", fmt.Errorf("version is required")
	}
	failureBackoff, maxFailureBackoff := resolveBackoffDurations(opts.FailureBackoff, opts.MaxFailureBackoff)

	return strings.Join([]string{
		quoteWindowsArg(opts.BinaryPath),
		"daemon",
		"--state-file", quoteWindowsArg(opts.StateFile),
		"--version", quoteWindowsArg(opts.Version),
		"--interval", quoteWindowsArg(opts.Interval.String()),
		"--failure-backoff", quoteWindowsArg(failureBackoff.String()),
		"--max-failure-backoff", quoteWindowsArg(maxFailureBackoff.String()),
		"--apply=" + strconv.FormatBool(opts.Apply),
	}, " "), nil
}

func InstallService(ctx context.Context, opts ServiceInstallOptions) error {
	cmdline, err := RenderServiceCommandLine(opts.Unit)
	if err != nil {
		return err
	}

	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := runner.Run(ctx, "sc.exe", "create", opts.Unit.ServiceName, "start=", "auto", "binPath=", cmdline, "DisplayName=", "Wiregate Windows Agent"); err != nil {
		return fmt.Errorf("sc.exe create: %w", err)
	}
	if err := runner.Run(ctx, "sc.exe", "start", opts.Unit.ServiceName); err != nil {
		return fmt.Errorf("sc.exe start: %w", err)
	}
	return nil
}

func UninstallService(ctx context.Context, serviceName string, runner CommandRunner) error {
	if err := validateServiceName(serviceName); err != nil {
		return err
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := runner.Run(ctx, "sc.exe", "stop", serviceName); err != nil {
		return fmt.Errorf("sc.exe stop: %w", err)
	}
	if err := runner.Run(ctx, "sc.exe", "delete", serviceName); err != nil {
		return fmt.Errorf("sc.exe delete: %w", err)
	}
	return nil
}

func validateServiceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("must be provided")
	}
	if !serviceNamePattern.MatchString(name) {
		return fmt.Errorf("contains unsafe characters")
	}
	return nil
}

func validateAbsolutePath(path string) error {
	if !isAbsolutePath(path) {
		return fmt.Errorf("must be absolute")
	}
	return nil
}

func quoteWindowsArg(arg string) string {
	return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
}

func isAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	return false
}

package linuxagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9@_.-]+\.service$`)

type ServiceUnitOptions struct {
	BinaryPath        string
	StateFile         string
	Version           string
	Interval          time.Duration
	FailureBackoff    time.Duration
	MaxFailureBackoff time.Duration
	Apply             bool
}

type ServiceInstallOptions struct {
	ServiceFilePath string
	Unit            ServiceUnitOptions
	Runner          CommandRunner
}

func RenderServiceUnit(opts ServiceUnitOptions) (string, error) {
	if err := validateAbsolutePath(opts.BinaryPath); err != nil {
		return "", fmt.Errorf("binary path: %w", err)
	}
	if err := validateAbsolutePath(opts.StateFile); err != nil {
		return "", fmt.Errorf("state file: %w", err)
	}
	if opts.Interval <= 0 {
		return "", fmt.Errorf("interval must be positive")
	}
	failureBackoff, maxFailureBackoff := resolveBackoffDurations(opts.FailureBackoff, opts.MaxFailureBackoff)
	if strings.TrimSpace(opts.Version) == "" {
		return "", fmt.Errorf("version is required")
	}

	execStart := renderSystemdArgs(
		opts.BinaryPath,
		"daemon",
		"--state-file", opts.StateFile,
		"--version", opts.Version,
		"--interval", opts.Interval.String(),
		"--failure-backoff", failureBackoff.String(),
		"--max-failure-backoff", maxFailureBackoff.String(),
		"--apply="+strconv.FormatBool(opts.Apply),
	)

	return strings.Join([]string{
		"[Unit]",
		"Description=wiregate linux agent",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"User=root",
		"Group=root",
		"ExecStart=" + execStart,
		"Restart=on-failure",
		"RestartSec=5",
		"PrivateTmp=true",
		"ProtectHome=true",
		"ReadWritePaths=/etc/wireguard /var/lib/wiregate-agent",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n"), nil
}

func InstallService(ctx context.Context, opts ServiceInstallOptions) error {
	if err := validateServiceFilePath(opts.ServiceFilePath); err != nil {
		return err
	}
	body, err := RenderServiceUnit(opts.Unit)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.ServiceFilePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(opts.ServiceFilePath, []byte(body), 0o644); err != nil {
		return err
	}

	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	serviceName := filepath.Base(opts.ServiceFilePath)
	if err := runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := runner.Run(ctx, "systemctl", "enable", "--now", serviceName); err != nil {
		return fmt.Errorf("systemctl enable --now: %w", err)
	}
	return nil
}

func UninstallService(ctx context.Context, serviceFilePath string, runner CommandRunner) error {
	if err := validateServiceFilePath(serviceFilePath); err != nil {
		return err
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	serviceName := filepath.Base(serviceFilePath)
	if err := runner.Run(ctx, "systemctl", "disable", "--now", serviceName); err != nil {
		return fmt.Errorf("systemctl disable --now: %w", err)
	}
	if err := os.Remove(serviceFilePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	return nil
}

func renderSystemdArgs(args ...string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return strings.Join(quoted, " ")
}

func validateAbsolutePath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("must be absolute")
	}
	return nil
}

func validateServiceFilePath(path string) error {
	if err := validateAbsolutePath(path); err != nil {
		return fmt.Errorf("service file path: %w", err)
	}
	if filepath.Ext(path) != ".service" {
		return fmt.Errorf("service file path must end with .service")
	}
	if !serviceNamePattern.MatchString(filepath.Base(path)) {
		return fmt.Errorf("unsafe systemd service name")
	}
	return nil
}

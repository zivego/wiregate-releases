package windowsagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

var ifacePattern = regexp.MustCompile(`^[a-zA-Z0-9_=+.-]{1,15}$`)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type ApplyOptions struct {
	IgnoreUninstallError bool
}

func ApplyConfig(ctx context.Context, runner CommandRunner, configPath string, opts ApplyOptions) error {
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := validateConfigPath(configPath); err != nil {
		return err
	}

	serviceName, err := tunnelServiceName(configPath)
	if err != nil {
		return err
	}
	if err := runner.Run(ctx, "wireguard.exe", "/uninstalltunnelservice", serviceName); err != nil && !opts.IgnoreUninstallError {
		return fmt.Errorf("wireguard.exe /uninstalltunnelservice: %w", err)
	}
	if err := runner.Run(ctx, "wireguard.exe", "/installtunnelservice", configPath); err != nil {
		return fmt.Errorf("wireguard.exe /installtunnelservice: %w", err)
	}
	return nil
}

func DisableConfig(ctx context.Context, runner CommandRunner, configPath string) error {
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := validateConfigPath(configPath); err != nil {
		return err
	}
	serviceName, err := tunnelServiceName(configPath)
	if err != nil {
		return err
	}
	if err := runner.Run(ctx, "wireguard.exe", "/uninstalltunnelservice", serviceName); err != nil {
		return fmt.Errorf("wireguard.exe /uninstalltunnelservice: %w", err)
	}
	return nil
}

func validateConfigPath(configPath string) error {
	if !isAbsolutePath(configPath) {
		return fmt.Errorf("config path must be absolute")
	}
	if filepath.Ext(configPath) != ".conf" {
		return fmt.Errorf("config path must end with .conf")
	}
	base := filepathBase(configPath[:len(configPath)-len(filepath.Ext(configPath))])
	if !ifacePattern.MatchString(base) {
		return fmt.Errorf("config filename must map to a safe interface name")
	}
	return nil
}

func filepathBase(path string) string {
	return filepath.Base(path)
}

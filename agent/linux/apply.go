package linuxagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	IgnoreDownError bool
}

func ApplyConfig(ctx context.Context, runner CommandRunner, configPath string, opts ApplyOptions) error {
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := validateConfigPath(configPath); err != nil {
		return err
	}

	if err := runner.Run(ctx, "wg-quick", "down", configPath); err != nil && !opts.IgnoreDownError {
		return fmt.Errorf("wg-quick down: %w", err)
	}
	if err := runner.Run(ctx, "wg-quick", "up", configPath); err != nil {
		return fmt.Errorf("wg-quick up: %w", err)
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
	if err := runner.Run(ctx, "wg-quick", "down", configPath); err != nil {
		return fmt.Errorf("wg-quick down: %w", err)
	}
	return nil
}

func validateConfigPath(configPath string) error {
	if !filepath.IsAbs(configPath) {
		return fmt.Errorf("config path must be absolute")
	}
	if filepath.Ext(configPath) != ".conf" {
		return fmt.Errorf("config path must end with .conf")
	}
	base := strings.TrimSuffix(filepath.Base(configPath), ".conf")
	if !ifacePattern.MatchString(base) {
		return fmt.Errorf("config filename must map to a safe interface name")
	}
	return nil
}

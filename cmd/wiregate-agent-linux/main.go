package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	linuxagent "github.com/zivego/wiregate/agent/linux"
	"github.com/zivego/wiregate/agent/shared/selfupdate"
)

const defaultVersion = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	switch os.Args[1] {
	case "enroll":
		if err := runEnroll(client, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "check-in":
		if err := runCheckIn(client, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "daemon":
		if err := runDaemon(client, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "install-service":
		if err := runInstallService(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "uninstall-service":
		if err := runUninstallService(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runEnroll(client *http.Client, args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	server := fs.String("server", "", "wiregate server URL")
	token := fs.String("token", "", "enrollment token")
	hostname := fs.String("hostname", "", "agent hostname")
	stateFile := fs.String("state-file", "/var/lib/wiregate-agent/state.json", "agent state path")
	privateKeyFile := fs.String("private-key-file", "/var/lib/wiregate-agent/private.key", "private key path")
	configFile := fs.String("config-file", "/etc/wireguard/wiregate.conf", "wireguard config path")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wg-quick")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *server == "" || *token == "" {
		return fmt.Errorf("server and token are required")
	}
	if *hostname == "" {
		localHostname, err := os.Hostname()
		if err != nil {
			return err
		}
		*hostname = localHostname
	}

	privateKey, publicKey, err := linuxagent.EnsurePrivateKey(*privateKeyFile)
	if err != nil {
		return err
	}
	state, wgCfg, err := linuxagent.Enroll(context.Background(), client, *server, *token, *hostname, publicKey)
	if err != nil {
		return err
	}
	state.PrivateKeyPath = *privateKeyFile
	state.ConfigPath = *configFile
	applyErr := linuxagent.WriteAndApplyConfig(context.Background(), &state, privateKey, wgCfg, *apply, nil)
	if err := linuxagent.SaveState(*stateFile, state); err != nil {
		return err
	}
	if applyErr != nil {
		return applyErr
	}
	fmt.Printf("enrolled agent %s\n", state.AgentID)
	return nil
}

func runCheckIn(client *http.Client, args []string) error {
	fs := flag.NewFlagSet("check-in", flag.ContinueOnError)
	stateFile := fs.String("state-file", "/var/lib/wiregate-agent/state.json", "agent state path")
	version := fs.String("version", defaultVersion, "agent version")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wg-quick")
	if err := fs.Parse(args); err != nil {
		return err
	}

	resp, err := syncOnce(client, *stateFile, *version, *apply)
	if err != nil {
		return err
	}
	fmt.Printf("checked in agent %s reconfigure_required=%t\n", resp.Agent.ID, resp.ReconfigureRequired)
	return nil
}

func runDaemon(client *http.Client, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	stateFile := fs.String("state-file", "/var/lib/wiregate-agent/state.json", "agent state path")
	version := fs.String("version", defaultVersion, "agent version")
	interval := fs.Duration("interval", 30*time.Second, "check-in interval")
	failureBackoff := fs.Duration("failure-backoff", 5*time.Second, "delay before retry after failed sync")
	maxFailureBackoff := fs.Duration("max-failure-backoff", time.Minute, "maximum delay between failed sync retries")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wg-quick")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return linuxagent.RunLoop(context.Background(), client, *stateFile, *interval, linuxagent.SyncOptions{
		Version:           *version,
		Apply:             *apply,
		FailureBackoff:    *failureBackoff,
		MaxFailureBackoff: *maxFailureBackoff,
		OnLoopError: func(err error, consecutiveFailures int, nextRetryIn time.Duration) {
			fmt.Fprintf(os.Stderr, "daemon sync failed: %v consecutive_failures=%d next_retry_in=%s\n", err, consecutiveFailures, nextRetryIn)
		},
		OnUpdateApplied: func(result selfupdate.Result) {
			fmt.Fprintf(os.Stdout, "agent updated to %s, exiting for restart\n", result.NewVersion)
			os.Exit(0) // systemd will restart with new binary
		},
	})
}

func runInstallService(args []string) error {
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	binaryPath := fs.String("binary-path", "/usr/local/bin/wiregate-agent-linux", "wiregate agent binary path")
	serviceFile := fs.String("service-file", "/etc/systemd/system/wiregate-agent-linux.service", "systemd unit path")
	stateFile := fs.String("state-file", "/var/lib/wiregate-agent/state.json", "agent state path")
	version := fs.String("version", defaultVersion, "agent version")
	interval := fs.Duration("interval", 30*time.Second, "check-in interval")
	failureBackoff := fs.Duration("failure-backoff", 5*time.Second, "delay before retry after failed sync")
	maxFailureBackoff := fs.Duration("max-failure-backoff", time.Minute, "maximum delay between failed sync retries")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wg-quick")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return linuxagent.InstallService(context.Background(), linuxagent.ServiceInstallOptions{
		ServiceFilePath: *serviceFile,
		Unit: linuxagent.ServiceUnitOptions{
			BinaryPath:        *binaryPath,
			StateFile:         *stateFile,
			Version:           *version,
			Interval:          *interval,
			FailureBackoff:    *failureBackoff,
			MaxFailureBackoff: *maxFailureBackoff,
			Apply:             *apply,
		},
	})
}

func runUninstallService(args []string) error {
	fs := flag.NewFlagSet("uninstall-service", flag.ContinueOnError)
	serviceFile := fs.String("service-file", "/etc/systemd/system/wiregate-agent-linux.service", "systemd unit path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return linuxagent.UninstallService(context.Background(), *serviceFile, nil)
}

func syncOnce(client *http.Client, stateFile, version string, apply bool) (linuxagent.CheckInResponse, error) {
	return linuxagent.SyncOnce(context.Background(), client, stateFile, linuxagent.SyncOptions{
		Version: version,
		Apply:   apply,
	})
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wiregate-agent-linux <enroll|check-in|daemon|install-service|uninstall-service> [flags]")
}

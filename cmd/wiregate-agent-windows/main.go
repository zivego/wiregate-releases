package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	windowsagent "github.com/zivego/wiregate/agent/windows"
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
	stateFile := fs.String("state-file", `C:\ProgramData\Wiregate\state.json`, "agent state path")
	privateKeyFile := fs.String("private-key-file", `C:\ProgramData\Wiregate\private.key`, "private key path")
	configFile := fs.String("config-file", `C:\ProgramData\Wiregate\wiregate.conf`, "wireguard config path")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wireguard.exe")
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

	privateKey, publicKey, err := windowsagent.EnsurePrivateKey(*privateKeyFile)
	if err != nil {
		return err
	}
	state, wgCfg, err := windowsagent.Enroll(context.Background(), client, *server, *token, *hostname, publicKey)
	if err != nil {
		return err
	}
	state.PrivateKeyPath = *privateKeyFile
	state.ConfigPath = *configFile
	if err := windowsagent.WriteAndApplyConfig(context.Background(), &state, privateKey, wgCfg, *apply, nil); err != nil {
		return err
	}
	if err := windowsagent.SaveState(*stateFile, state); err != nil {
		return err
	}
	fmt.Printf("enrolled agent %s\n", state.AgentID)
	return nil
}

func runCheckIn(client *http.Client, args []string) error {
	fs := flag.NewFlagSet("check-in", flag.ContinueOnError)
	stateFile := fs.String("state-file", `C:\ProgramData\Wiregate\state.json`, "agent state path")
	version := fs.String("version", defaultVersion, "agent version")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wireguard.exe")
	if err := fs.Parse(args); err != nil {
		return err
	}

	resp, err := windowsagent.SyncOnce(context.Background(), client, *stateFile, windowsagent.SyncOptions{
		Version: *version,
		Apply:   *apply,
	})
	if err != nil {
		return err
	}
	fmt.Printf("checked in agent %s reconfigure_required=%t\n", resp.Agent.ID, resp.ReconfigureRequired)
	return nil
}

func runDaemon(client *http.Client, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	stateFile := fs.String("state-file", `C:\ProgramData\Wiregate\state.json`, "agent state path")
	version := fs.String("version", defaultVersion, "agent version")
	interval := fs.Duration("interval", 30*time.Second, "check-in interval")
	failureBackoff := fs.Duration("failure-backoff", 5*time.Second, "delay before retry after failed sync")
	maxFailureBackoff := fs.Duration("max-failure-backoff", time.Minute, "maximum delay between failed sync retries")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wireguard.exe")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return windowsagent.RunLoop(context.Background(), client, *stateFile, *interval, windowsagent.SyncOptions{
		Version:           *version,
		Apply:             *apply,
		FailureBackoff:    *failureBackoff,
		MaxFailureBackoff: *maxFailureBackoff,
		OnLoopError: func(err error, consecutiveFailures int, nextRetryIn time.Duration) {
			fmt.Fprintf(os.Stderr, "daemon sync failed: %v consecutive_failures=%d next_retry_in=%s\n", err, consecutiveFailures, nextRetryIn)
		},
		OnUpdateApplied: func(result selfupdate.Result) {
			fmt.Fprintf(os.Stdout, "agent updated to %s, exiting for restart\n", result.NewVersion)
			os.Exit(0)
		},
	})
}

func runInstallService(args []string) error {
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	serviceName := fs.String("service-name", "WiregateAgent", "windows service name")
	binaryPath := fs.String("binary-path", `C:\Program Files\Wiregate\wiregate-agent-windows.exe`, "wiregate agent binary path")
	stateFile := fs.String("state-file", `C:\ProgramData\Wiregate\state.json`, "agent state path")
	version := fs.String("version", defaultVersion, "agent version")
	interval := fs.Duration("interval", 30*time.Second, "check-in interval")
	failureBackoff := fs.Duration("failure-backoff", 5*time.Second, "delay before retry after failed sync")
	maxFailureBackoff := fs.Duration("max-failure-backoff", time.Minute, "maximum delay between failed sync retries")
	apply := fs.Bool("apply", true, "apply rendered WireGuard config with wireguard.exe")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return windowsagent.InstallService(context.Background(), windowsagent.ServiceInstallOptions{
		Unit: windowsagent.ServiceUnitOptions{
			ServiceName:       *serviceName,
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
	serviceName := fs.String("service-name", "WiregateAgent", "windows service name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return windowsagent.UninstallService(context.Background(), *serviceName, nil)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wiregate-agent-windows <enroll|check-in|daemon|install-service|uninstall-service> [flags]")
}

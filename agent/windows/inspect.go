package windowsagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zivego/wiregate/pkg/wgconfig"
)

type OutputRunner interface {
	Output(ctx context.Context, name string, args ...string) (string, error)
}

type ExecOutputRunner struct{}

func (ExecOutputRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type RuntimeObservation struct {
	InterfaceName     string
	InterfacePresent  bool
	ConfigFingerprint string
}

func InspectRuntime(ctx context.Context, runner OutputRunner, configPath string) (RuntimeObservation, error) {
	if runner == nil {
		runner = ExecOutputRunner{}
	}

	ifaceName, err := interfaceNameFromConfigPath(configPath)
	if err != nil {
		return RuntimeObservation{}, err
	}
	serviceName, err := tunnelServiceName(configPath)
	if err != nil {
		return RuntimeObservation{}, err
	}

	serviceOut, err := runner.Output(ctx, "sc.exe", "query", serviceName)
	if err != nil {
		if missingServiceOutput(serviceOut) {
			return RuntimeObservation{InterfaceName: ifaceName, InterfacePresent: false}, nil
		}
		return RuntimeObservation{}, fmt.Errorf("sc.exe query: %w", err)
	}
	if !serviceRunning(serviceOut) {
		return RuntimeObservation{InterfaceName: ifaceName, InterfacePresent: false}, nil
	}

	interfacesOut, err := runner.Output(ctx, "wg.exe", "show", "interfaces")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg.exe show interfaces: %w", err)
	}
	if !containsInterface(interfacesOut, ifaceName) {
		return RuntimeObservation{InterfaceName: ifaceName, InterfacePresent: false}, nil
	}

	peersOut, err := runner.Output(ctx, "wg.exe", "show", ifaceName, "peers")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg.exe show peers: %w", err)
	}
	endpointsOut, err := runner.Output(ctx, "wg.exe", "show", ifaceName, "endpoints")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg.exe show endpoints: %w", err)
	}
	allowedIPsOut, err := runner.Output(ctx, "wg.exe", "show", ifaceName, "allowed-ips")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg.exe show allowed-ips: %w", err)
	}
	address, err := parseConfigAddress(configPath)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("parse config address: %w", err)
	}

	publicKey := firstNonEmptyLine(peersOut)
	endpoint := parseFirstValue(endpointsOut)
	allowedIPs := parseCommaSeparatedValues(parseFirstValue(allowedIPsOut))

	return RuntimeObservation{
		InterfaceName:     ifaceName,
		InterfacePresent:  true,
		ConfigFingerprint: wgconfig.Fingerprint(address, endpoint, publicKey, allowedIPs, nil, nil),
	}, nil
}

func missingServiceOutput(body string) bool {
	body = strings.ToLower(body)
	return strings.Contains(body, "1060") || strings.Contains(body, "does not exist as an installed service")
}

func serviceRunning(body string) bool {
	body = strings.ToUpper(body)
	return strings.Contains(body, "STATE") && strings.Contains(body, "RUNNING")
}

func containsInterface(body, ifaceName string) bool {
	for _, field := range strings.Fields(body) {
		if field == ifaceName {
			return true
		}
	}
	return false
}

func firstNonEmptyLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func parseFirstValue(body string) string {
	line := firstNonEmptyLine(body)
	if line == "" {
		return ""
	}
	if strings.Contains(line, "\t") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) == 2 {
			return strings.TrimSpace(fields[1])
		}
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(fields[1:], " "))
}

func parseCommaSeparatedValues(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	normalized := strings.ReplaceAll(body, ",", " ")
	parts := strings.Fields(normalized)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func parseConfigAddress(configPath string) (string, error) {
	body, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "address") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		return strings.TrimSpace(parts[1]), nil
	}
	return "", nil
}

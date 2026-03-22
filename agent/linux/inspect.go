package linuxagent

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zivego/wiregate/pkg/wgconfig"
)

type OutputRunner interface {
	Output(ctx context.Context, name string, args ...string) (string, error)
}

type ExecOutputRunner struct{}

func (ExecOutputRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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

	interfacesOut, err := runner.Output(ctx, "wg", "show", "interfaces")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg show interfaces: %w", err)
	}
	if !containsInterface(interfacesOut, ifaceName) {
		return RuntimeObservation{InterfaceName: ifaceName, InterfacePresent: false}, nil
	}

	peersOut, err := runner.Output(ctx, "wg", "show", ifaceName, "peers")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg show peers: %w", err)
	}
	endpointsOut, err := runner.Output(ctx, "wg", "show", ifaceName, "endpoints")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg show endpoints: %w", err)
	}
	allowedIPsOut, err := runner.Output(ctx, "wg", "show", ifaceName, "allowed-ips")
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("wg show allowed-ips: %w", err)
	}
	addressesOut, err := runner.Output(ctx, "ip", "-o", "-4", "addr", "show", "dev", ifaceName)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("ip addr show: %w", err)
	}

	publicKey := firstNonEmptyLine(peersOut)
	endpoint := parseFirstValue(endpointsOut)
	allowedIPs := parseCommaSeparatedValues(parseFirstValue(allowedIPsOut))
	address := parseIPv4Address(addressesOut)

	return RuntimeObservation{
		InterfaceName:     ifaceName,
		InterfacePresent:  true,
		ConfigFingerprint: wgconfig.Fingerprint(address, endpoint, publicKey, allowedIPs, nil, nil),
	}, nil
}

func interfaceNameFromConfigPath(configPath string) (string, error) {
	if err := validateConfigPath(configPath); err != nil {
		return "", err
	}
	return strings.TrimSuffix(filepath.Base(configPath), ".conf"), nil
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

func parseIPv4Address(body string) string {
	for _, field := range strings.Fields(body) {
		if strings.Count(field, ".") == 3 && strings.Contains(field, "/") {
			return field
		}
	}
	return ""
}

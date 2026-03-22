package wgconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

func Fingerprint(interfaceAddress, serverEndpoint, serverPublicKey string, allowedIPs, dnsServers, dnsSearchDomains []string) string {
	if strings.TrimSpace(interfaceAddress) == "" || strings.TrimSpace(serverPublicKey) == "" || len(allowedIPs) == 0 {
		return ""
	}

	normalizedAllowedIPs := slices.Clone(allowedIPs)
	slices.Sort(normalizedAllowedIPs)
	normalizedDNSServers := slices.Clone(dnsServers)
	slices.Sort(normalizedDNSServers)
	normalizedDNSSearchDomains := slices.Clone(dnsSearchDomains)
	slices.Sort(normalizedDNSSearchDomains)

	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(interfaceAddress),
		strings.TrimSpace(serverEndpoint),
		strings.TrimSpace(serverPublicKey),
		strings.Join(normalizedAllowedIPs, ","),
		strings.Join(normalizedDNSServers, ","),
		strings.Join(normalizedDNSSearchDomains, ","),
	}, "\n")))
	return hex.EncodeToString(sum[:])
}

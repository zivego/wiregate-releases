// Package kernel implements the WireGuard adapter using the OS kernel
// via the wgctrl library (netlink on Linux, userspace on other platforms).
package kernel

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"sync"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/zivego/wiregate/pkg/wgadapter"
)

// Adapter talks to a real WireGuard interface through the kernel (netlink).
type Adapter struct {
	iface string

	mu          sync.Mutex
	peerIDByKey map[wgtypes.Key]string // PublicKey → PeerID mapping
}

// New creates a kernel adapter for the given WireGuard interface name (e.g. "wg0").
func New(interfaceName string) *Adapter {
	return &Adapter{
		iface:       interfaceName,
		peerIDByKey: make(map[wgtypes.Key]string),
	}
}

func (a *Adapter) Ping(_ context.Context) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wgctrl open: %w", err)
	}
	defer client.Close()

	_, err = client.Device(a.iface)
	if err != nil {
		return fmt.Errorf("wg device %q: %w", a.iface, err)
	}
	return nil
}

func (a *Adapter) ListPeers(_ context.Context) ([]wgadapter.PeerState, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("wgctrl open: %w", err)
	}
	defer client.Close()

	dev, err := client.Device(a.iface)
	if err != nil {
		return nil, fmt.Errorf("wg device %q: %w", a.iface, err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]wgadapter.PeerState, 0, len(dev.Peers))
	for _, p := range dev.Peers {
		pubKeyB64 := base64.StdEncoding.EncodeToString(p.PublicKey[:])

		peerID := a.peerIDByKey[p.PublicKey]
		if peerID == "" {
			// Peer exists in kernel but was not applied through us.
			// Use the base64-encoded public key as a fallback identifier.
			peerID = pubKeyB64
		}

		allowedIPs := make([]string, 0, len(p.AllowedIPs))
		for _, ipNet := range p.AllowedIPs {
			allowedIPs = append(allowedIPs, ipNet.String())
		}

		out = append(out, wgadapter.PeerState{
			PeerID:     peerID,
			PublicKey:  pubKeyB64,
			AllowedIPs: allowedIPs,
			Status:     "configured",
		})
	}
	return out, nil
}

func (a *Adapter) ApplyPeer(_ context.Context, in wgadapter.ApplyPeerInput) error {
	pubKey, err := parseKey(in.PublicKey)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}

	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wgctrl open: %w", err)
	}
	defer client.Close()

	a.mu.Lock()
	defer a.mu.Unlock()

	if in.Action == "remove" {
		delete(a.peerIDByKey, pubKey)
		return client.ConfigureDevice(a.iface, wgtypes.Config{
			Peers: []wgtypes.PeerConfig{
				{
					PublicKey: pubKey,
					Remove:   true,
				},
			},
		})
	}

	allowedIPs, err := parseCIDRs(in.AllowedIPs)
	if err != nil {
		return err
	}

	a.peerIDByKey[pubKey] = in.PeerID

	return client.ConfigureDevice(a.iface, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey:         pubKey,
				ReplaceAllowedIPs: true,
				AllowedIPs:        allowedIPs,
			},
		},
	})
}

// RegisterPeerMapping adds a PublicKey → PeerID mapping without touching the
// kernel. This is useful on startup to pre-populate the mapping from the
// database so that ListPeers can return correct PeerIDs immediately.
func (a *Adapter) RegisterPeerMapping(publicKeyB64, peerID string) error {
	key, err := parseKey(publicKeyB64)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.peerIDByKey[key] = peerID
	a.mu.Unlock()
	return nil
}

func parseKey(b64 string) (wgtypes.Key, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("invalid base64 key: %w", err)
	}
	if len(raw) != wgtypes.KeyLen {
		return wgtypes.Key{}, fmt.Errorf("key length %d, want %d", len(raw), wgtypes.KeyLen)
	}
	var key wgtypes.Key
	copy(key[:], raw)
	return key, nil
}

func parseCIDRs(cidrs []string) ([]net.IPNet, error) {
	out := make([]net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse CIDR %q: %w", cidr, err)
		}
		out = append(out, *ipNet)
	}
	return out, nil
}

package wgadapter

import (
	"context"
	"errors"
)

var ErrNotImplemented = errors.New("wgadapter: not implemented")

type PeerState struct {
	PeerID     string
	PublicKey  string
	AllowedIPs []string
	Status     string
}

type ApplyPeerInput struct {
	PeerID     string
	PublicKey  string
	AllowedIPs []string
	Action     string
}

// Adapter isolates all WireGuard runtime interactions.
type Adapter interface {
	Ping(ctx context.Context) error
	ListPeers(ctx context.Context) ([]PeerState, error)
	ApplyPeer(ctx context.Context, in ApplyPeerInput) error
}

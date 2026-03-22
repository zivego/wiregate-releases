package fake

import (
	"context"
	"sync"

	"github.com/zivego/wiregate/pkg/wgadapter"
)

// Adapter is an in-memory test double used by the MVP scaffold.
type Adapter struct {
	mu    sync.Mutex
	peers map[string]wgadapter.PeerState
}

func New() *Adapter {
	return &Adapter{peers: map[string]wgadapter.PeerState{}}
}

func (a *Adapter) Ping(_ context.Context) error {
	return nil
}

func (a *Adapter) ListPeers(_ context.Context) ([]wgadapter.PeerState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]wgadapter.PeerState, 0, len(a.peers))
	for _, peer := range a.peers {
		out = append(out, peer)
	}
	return out, nil
}

func (a *Adapter) ApplyPeer(_ context.Context, in wgadapter.ApplyPeerInput) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if in.Action == "remove" {
		delete(a.peers, in.PeerID)
		return nil
	}

	a.peers[in.PeerID] = wgadapter.PeerState{
		PeerID:     in.PeerID,
		PublicKey:  in.PublicKey,
		AllowedIPs: in.AllowedIPs,
		Status:     "configured",
	}
	return nil
}

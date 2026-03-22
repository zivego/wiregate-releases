package wgcontrol

import (
	"context"

	"github.com/zivego/wiregate/pkg/wgadapter"
)

// Service is the control-plane boundary for WireGuard runtime operations.
type Service struct {
	adapter wgadapter.Adapter
}

func NewService(adapter wgadapter.Adapter) *Service {
	return &Service{adapter: adapter}
}

func (s *Service) Ping(ctx context.Context) error {
	return s.adapter.Ping(ctx)
}

func (s *Service) ListPeers(ctx context.Context) ([]wgadapter.PeerState, error) {
	return s.adapter.ListPeers(ctx)
}

// ApplyPeer delegates to adapter. Real runtime mutation is intentionally deferred.
func (s *Service) ApplyPeer(ctx context.Context, in wgadapter.ApplyPeerInput) error {
	return s.adapter.ApplyPeer(ctx, in)
}

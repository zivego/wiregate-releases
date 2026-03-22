// Package ipam provides IP address management with multi-pool and IPv6 support.
package ipam

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/zivego/wiregate/internal/persistence/ipamrepo"
)

// Service manages IPAM pools and address allocation.
type Service struct {
	repo *ipamrepo.Repo
}

// NewService creates a new IPAM service.
func NewService(repo *ipamrepo.Repo) *Service {
	return &Service{repo: repo}
}

// PoolSummary is the API representation of an address pool with usage info.
type PoolSummary struct {
	ID          string `json:"id"`
	CIDR        string `json:"cidr"`
	Description string `json:"description"`
	IsIPv6      bool   `json:"is_ipv6"`
	Gateway     string `json:"gateway,omitempty"`
	TotalAddrs  int    `json:"total_addresses"`
	UsedAddrs   int    `json:"used_addresses"`
	FreeAddrs   int    `json:"free_addresses"`
	CreatedAt   string `json:"created_at"`
}

// ReservationInfo is the API representation of a reservation.
type ReservationInfo struct {
	ID         string `json:"id"`
	PoolID     string `json:"pool_id"`
	Address    string `json:"address"`
	PeerID     string `json:"peer_id,omitempty"`
	Label      string `json:"label,omitempty"`
	ReservedAt string `json:"reserved_at"`
}

// CreatePool validates and creates a new address pool.
func (s *Service) CreatePool(ctx context.Context, id, cidr, description, gateway string) (*ipamrepo.Pool, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}
	// Normalize to masked form.
	prefix = prefix.Masked()
	normalizedCIDR := prefix.String()

	existing, err := s.repo.FindPoolByCIDR(ctx, normalizedCIDR)
	if err != nil {
		return nil, fmt.Errorf("ipam check existing pool: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("pool with CIDR %s already exists", normalizedCIDR)
	}

	if gateway != "" {
		gw, err := netip.ParseAddr(gateway)
		if err != nil {
			return nil, fmt.Errorf("invalid gateway address: %w", err)
		}
		if !prefix.Contains(gw) {
			return nil, fmt.Errorf("gateway %s is not within pool CIDR %s", gateway, normalizedCIDR)
		}
	}

	now := time.Now().UTC()
	pool := ipamrepo.Pool{
		ID:          id,
		CIDR:        normalizedCIDR,
		Description: description,
		IsIPv6:      prefix.Addr().Is6() && !prefix.Addr().Is4In6(),
		Gateway:     gateway,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.InsertPool(ctx, pool); err != nil {
		return nil, err
	}
	return &pool, nil
}

// ListPools returns all pools with usage statistics.
func (s *Service) ListPools(ctx context.Context) ([]PoolSummary, error) {
	pools, err := s.repo.ListPools(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]PoolSummary, 0, len(pools))
	for _, p := range pools {
		used, err := s.repo.CountByPool(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		total := poolSize(p.CIDR)
		summaries = append(summaries, PoolSummary{
			ID:          p.ID,
			CIDR:        p.CIDR,
			Description: p.Description,
			IsIPv6:      p.IsIPv6,
			Gateway:     p.Gateway,
			TotalAddrs:  total,
			UsedAddrs:   used,
			FreeAddrs:   total - used,
			CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return summaries, nil
}

// GetPool returns a single pool summary.
func (s *Service) GetPool(ctx context.Context, poolID string) (*PoolSummary, error) {
	p, err := s.repo.FindPoolByID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	used, err := s.repo.CountByPool(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	total := poolSize(p.CIDR)
	return &PoolSummary{
		ID:          p.ID,
		CIDR:        p.CIDR,
		Description: p.Description,
		IsIPv6:      p.IsIPv6,
		Gateway:     p.Gateway,
		TotalAddrs:  total,
		UsedAddrs:   used,
		FreeAddrs:   total - used,
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

// DeletePool removes a pool and all its reservations.
func (s *Service) DeletePool(ctx context.Context, poolID string) error {
	return s.repo.DeletePool(ctx, poolID)
}

// AllocateAddress finds the next free address in a pool and reserves it.
func (s *Service) AllocateAddress(ctx context.Context, resID, poolID, peerID, label string) (*ReservationInfo, error) {
	pool, err := s.repo.FindPoolByID(ctx, poolID)
	if err != nil {
		return nil, fmt.Errorf("ipam allocate find pool: %w", err)
	}
	if pool == nil {
		return nil, fmt.Errorf("pool %s not found", poolID)
	}

	prefix, err := netip.ParsePrefix(pool.CIDR)
	if err != nil {
		return nil, fmt.Errorf("ipam allocate parse cidr: %w", err)
	}

	usedAddrs, err := s.repo.PoolUsage(ctx, poolID)
	if err != nil {
		return nil, fmt.Errorf("ipam allocate usage: %w", err)
	}
	usedSet := make(map[netip.Addr]struct{}, len(usedAddrs))
	for _, a := range usedAddrs {
		addr, err := netip.ParseAddr(a)
		if err != nil {
			continue
		}
		usedSet[addr] = struct{}{}
	}

	// Also skip the gateway.
	if pool.Gateway != "" {
		if gw, err := netip.ParseAddr(pool.Gateway); err == nil {
			usedSet[gw] = struct{}{}
		}
	}

	// Find next free address (skip network address for IPv4).
	base := prefix.Masked().Addr()
	for candidate := base.Next(); prefix.Contains(candidate); candidate = candidate.Next() {
		if _, exists := usedSet[candidate]; exists {
			continue
		}
		// Skip broadcast for IPv4 (/31 and /32 are point-to-point, no broadcast).
		if prefix.Addr().Is4() && prefix.Bits() < 31 {
			broadcast := lastAddr(prefix)
			if candidate == broadcast {
				continue
			}
		}

		now := time.Now().UTC()
		suffix := "/32"
		if prefix.Addr().Is6() && !prefix.Addr().Is4In6() {
			suffix = "/128"
		}
		res := ipamrepo.Reservation{
			ID:         resID,
			PoolID:     poolID,
			Address:    candidate.String() + suffix,
			PeerID:     peerID,
			Label:      label,
			ReservedAt: now,
		}
		if err := s.repo.InsertReservation(ctx, res); err != nil {
			return nil, fmt.Errorf("ipam allocate insert: %w", err)
		}
		return &ReservationInfo{
			ID:         res.ID,
			PoolID:     res.PoolID,
			Address:    res.Address,
			PeerID:     res.PeerID,
			Label:      res.Label,
			ReservedAt: now.Format(time.RFC3339),
		}, nil
	}

	return nil, fmt.Errorf("no free addresses in pool %s (%s)", poolID, pool.CIDR)
}

// ReleaseByPeer releases all reservations for a given peer.
func (s *Service) ReleaseByPeer(ctx context.Context, peerID string) error {
	return s.repo.DeleteReservationByPeerID(ctx, peerID)
}

// ListReservations returns all reservations for a pool.
func (s *Service) ListReservations(ctx context.Context, poolID string) ([]ReservationInfo, error) {
	reservations, err := s.repo.ListReservations(ctx, poolID)
	if err != nil {
		return nil, err
	}
	infos := make([]ReservationInfo, 0, len(reservations))
	for _, r := range reservations {
		infos = append(infos, ReservationInfo{
			ID:         r.ID,
			PoolID:     r.PoolID,
			Address:    r.Address,
			PeerID:     r.PeerID,
			Label:      r.Label,
			ReservedAt: r.ReservedAt.UTC().Format(time.RFC3339),
		})
	}
	return infos, nil
}

// ReleaseReservation releases a single reservation by ID.
func (s *Service) ReleaseReservation(ctx context.Context, resID string) error {
	return s.repo.DeleteReservation(ctx, resID)
}

// poolSize returns usable host addresses in a CIDR prefix.
func poolSize(cidr string) int {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return 0
	}
	bits := prefix.Addr().BitLen() // 32 for IPv4, 128 for IPv6
	hostBits := bits - prefix.Bits()
	if hostBits <= 0 {
		return 1
	}
	if hostBits > 20 {
		// Cap to avoid huge numbers for large IPv6 pools.
		return 1 << 20
	}
	total := 1 << hostBits
	// Subtract network and broadcast for IPv4 (except /31, /32).
	if prefix.Addr().Is4() && hostBits >= 2 {
		total -= 2
	}
	return total
}

// lastAddr returns the last (broadcast) address in a prefix.
func lastAddr(prefix netip.Prefix) netip.Addr {
	addr := prefix.Masked().Addr()
	bits := prefix.Bits()
	addrLen := addr.BitLen()

	raw := addr.As16()
	for i := bits; i < addrLen; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		raw[byteIdx] |= 1 << bitIdx
	}
	if addr.Is4() {
		return netip.AddrFrom4([4]byte(raw[12:16]))
	}
	return netip.AddrFrom16(raw)
}

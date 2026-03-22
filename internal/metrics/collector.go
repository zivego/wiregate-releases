package metrics

import (
	"context"
	"log"
	"time"

	"github.com/zivego/wiregate/internal/reconcile"
	"github.com/zivego/wiregate/internal/wgcontrol"
	"github.com/zivego/wiregate/pkg/wgadapter"
)

// BusinessMetricsSource provides the data needed to update business gauges.
type BusinessMetricsSource struct {
	WG        *wgcontrol.Service
	Reconcile *reconcile.Service
	Counts    CountsProvider
}

// CountsProvider returns aggregate counts from the database.
type CountsProvider interface {
	AgentCountsByStatus(ctx context.Context) (map[string]int, error)
	PeerCountsByStatus(ctx context.Context) (map[string]int, error)
	ActiveSessionCount(ctx context.Context) (int, error)
	EnrollmentTokenCountsByStatus(ctx context.Context) (map[string]int, error)
}

// StartBackgroundCollector starts a goroutine that refreshes business metrics
// at the given interval. It stops when the context is cancelled.
func StartBackgroundCollector(ctx context.Context, src BusinessMetricsSource, interval time.Duration, logger *log.Logger) {
	go func() {
		// Collect once immediately on start.
		collect(ctx, src, logger)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collect(ctx, src, logger)
			}
		}
	}()
}

func collect(ctx context.Context, src BusinessMetricsSource, logger *log.Logger) {
	// WireGuard adapter metrics.
	if src.WG != nil {
		if err := src.WG.Ping(ctx); err != nil {
			WGPingSuccess.Set(0)
		} else {
			WGPingSuccess.Set(1)
		}

		peers, err := src.WG.ListPeers(ctx)
		if err == nil {
			WGPeersRuntime.Set(float64(len(peers)))
		}
	}

	// Reconcile drift.
	if src.Reconcile != nil {
		summary, err := src.Reconcile.Summarize(ctx)
		if err == nil {
			ReconcileDrifted.Set(float64(summary.Drifted))
		}
	}

	// Database counts.
	if src.Counts != nil {
		if counts, err := src.Counts.AgentCountsByStatus(ctx); err == nil {
			for status, n := range counts {
				AgentsTotal.WithLabelValues(status).Set(float64(n))
			}
		} else {
			logger.Printf("metrics: agent counts: %v", err)
		}

		if counts, err := src.Counts.PeerCountsByStatus(ctx); err == nil {
			for status, n := range counts {
				PeersTotal.WithLabelValues(status).Set(float64(n))
			}
		} else {
			logger.Printf("metrics: peer counts: %v", err)
		}

		if n, err := src.Counts.ActiveSessionCount(ctx); err == nil {
			SessionsActive.Set(float64(n))
		}

		if counts, err := src.Counts.EnrollmentTokenCountsByStatus(ctx); err == nil {
			for status, n := range counts {
				EnrollmentTokensTotal.WithLabelValues(status).Set(float64(n))
			}
		}
	}
}

// ObserveWGAdapterCall records the result of a WireGuard adapter operation.
// Can be called inline from reconcile or other services.
func ObserveWGAdapterCall(peers []wgadapter.PeerState) {
	WGPeersRuntime.Set(float64(len(peers)))
}

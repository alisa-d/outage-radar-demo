package ingest

import (
	"context"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics are the adapter metrics, all labelled by provider.
type Metrics struct {
	fetch      metric.Int64Counter
	duration   metric.Float64Histogram
	found      metric.Int64Gauge
	failures   metric.Int64Counter
	unresolved metric.Int64Counter
}

var healthValue = map[string]int64{"healthy": 0, "stale": 1, "suspicious": 2, "unknown": -1}

func NewMetrics(m metric.Meter, pool *pgxpool.Pool, clk clock.Clock) (*Metrics, error) {
	var x Metrics
	var err error
	if x.fetch, err = m.Int64Counter("outage_radar_fetch", metric.WithDescription("Fetches by result (ok|error)")); err != nil {
		return nil, err
	}
	if x.duration, err = m.Float64Histogram("outage_radar_fetch_duration", metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if x.found, err = m.Int64Gauge("outage_radar_notices_found", metric.WithDescription("Notices in the last successful fetch")); err != nil {
		return nil, err
	}
	if x.failures, err = m.Int64Counter("outage_radar_extraction_failures"); err != nil {
		return nil, err
	}
	if x.unresolved, err = m.Int64Counter("outage_radar_unresolved_names", metric.WithDescription("Outage Settlement names not in the Gazetteer")); err != nil {
		return nil, err
	}
	_, err = m.Int64ObservableGauge("outage_radar_provider_health", metric.WithDescription("0 healthy, 1 stale, 2 suspicious, -1 unknown"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			hs, err := Healths(ctx, pool, clk.Now())
			for name, h := range hs {
				o.Observe(healthValue[h.State], metric.WithAttributes(attribute.String("provider", name), attribute.String("state", h.State)))
			}
			return err
		}))
	return &x, err
}

func (m *Metrics) fetched(ctx context.Context, provider string, took time.Duration, err error) {
	if m == nil {
		return
	}
	res := "ok"
	if err != nil {
		res = "error"
	}
	p := attribute.String("provider", provider)
	m.fetch.Add(ctx, 1, metric.WithAttributes(p, attribute.String("result", res)))
	m.duration.Record(ctx, took.Seconds(), metric.WithAttributes(p))
}

func (m *Metrics) parsed(ctx context.Context, provider string, notices, failed, unresolved int) {
	if m == nil {
		return
	}
	p := metric.WithAttributes(attribute.String("provider", provider))
	m.found.Record(ctx, int64(notices), p)
	m.failures.Add(ctx, int64(failed), p)
	m.unresolved.Add(ctx, int64(unresolved), p)
}

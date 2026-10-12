package telemetry

import (
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NewRegistry returns a registry with the Go runtime (goroutines, GC pauses,
// heap) and process (CPU, memory) collectors already in it.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// HTTPMetrics are the API's request metrics, labeled by route pattern (never
// by raw URL, so /v1/posts/1 and /v1/posts/2 are one series).
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewHTTPMetrics registers the request metrics on reg.
func NewHTTPMetrics(reg prometheus.Registerer) *HTTPMetrics {
	m := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stresslab_http_requests_total",
			Help: "HTTP requests by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "stresslab_http_request_duration_seconds",
			Help: "HTTP request latency by method and route pattern.",
			// 0.5 s and 1 s are the latency SLOs' boundaries.
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, .75, 1, 2.5, 5},
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "stresslab_http_requests_in_flight",
			Help: "HTTP requests being served right now.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight)
	return m
}

// Start marks a request in flight; call the returned func when it's done.
func (m *HTTPMetrics) Start() func(method, route string, status int, d time.Duration) {
	m.inFlight.Inc()
	return func(method, route string, status int, d time.Duration) {
		m.inFlight.Dec()
		m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
		m.duration.WithLabelValues(method, route).Observe(d.Seconds())
	}
}

// PoolCollector exports the pgx pool's statistics. Pool wait — time spent
// waiting because every connection was busy — is what separates waiting *for*
// the database from waiting *on* it (METHODOLOGY, "Finding what broke").
type PoolCollector struct {
	pool *pgxpool.Pool

	acquires, emptyAcquires, canceled    *prometheus.Desc
	acquireSeconds, emptyWaitSeconds     *prometheus.Desc
	acquired, idle, total, max, newConns *prometheus.Desc
}

// NewPoolCollector returns a collector for pool; register it on a registry.
func NewPoolCollector(pool *pgxpool.Pool) *PoolCollector {
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("stresslab_db_pool_"+name, help, nil, nil)
	}
	return &PoolCollector{
		pool:             pool,
		acquires:         d("acquires_total", "Connections acquired from the pool."),
		emptyAcquires:    d("empty_acquires_total", "Acquires that had to wait because no connection was free."),
		canceled:         d("canceled_acquires_total", "Acquires canceled before a connection was free."),
		acquireSeconds:   d("acquire_seconds_total", "Total time spent acquiring connections."),
		emptyWaitSeconds: d("empty_acquire_wait_seconds_total", "Total time spent waiting for a busy pool: the pool wait."),
		acquired:         d("acquired_connections", "Connections in use right now."),
		idle:             d("idle_connections", "Idle connections right now."),
		total:            d("total_connections", "Open connections right now."),
		max:              d("max_connections", "The pool's size limit."),
		newConns:         d("new_connections_total", "Connections opened since start."),
	}
}

func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.acquires, c.emptyAcquires, c.canceled,
		c.acquireSeconds, c.emptyWaitSeconds, c.acquired, c.idle, c.total, c.max, c.newConns} {
		ch <- d
	}
}

func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}
	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter(c.acquires, float64(s.AcquireCount()))
	counter(c.emptyAcquires, float64(s.EmptyAcquireCount()))
	counter(c.canceled, float64(s.CanceledAcquireCount()))
	counter(c.acquireSeconds, s.AcquireDuration().Seconds())
	counter(c.emptyWaitSeconds, s.EmptyAcquireWaitTime().Seconds())
	counter(c.newConns, float64(s.NewConnsCount()))
	gauge(c.acquired, float64(s.AcquiredConns()))
	gauge(c.idle, float64(s.IdleConns()))
	gauge(c.total, float64(s.TotalConns()))
	gauge(c.max, float64(s.MaxConns()))
}

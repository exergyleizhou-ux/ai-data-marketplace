// Package metrics exposes Prometheus instrumentation: per-request HTTP counters
// and latency histograms, plus the default Go/process collectors, served at
// /metrics. Labels use the route template (c.FullPath, e.g. /datasets/:id) — not
// the raw URL — to keep cardinality bounded.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace", Subsystem: "http",
		Name: "requests_total", Help: "Total HTTP requests by method, route and status.",
	}, []string{"method", "route", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "marketplace", Subsystem: "http",
		Name: "request_duration_seconds", Help: "HTTP request latency by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	qualityJobs = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace", Subsystem: "quality",
		Name: "jobs_total", Help: "Quality-check jobs by resulting dataset status (reviewing/draft).",
	}, []string{"outcome"})

	// Compute-to-Data (C2D) sandbox jobs (design §17 observability).
	computeJobs = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace", Subsystem: "compute",
		Name: "jobs_total", Help: "C2D jobs by terminal/transition outcome (released/failed/rejected/review_pending).",
	}, []string{"outcome"})

	computeJobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "marketplace", Subsystem: "compute",
		Name: "job_duration_seconds", Help: "C2D job run time (claim → terminal) by output kind.",
		Buckets: []float64{0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"kind"})

	computeReclaims = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "marketplace", Subsystem: "compute",
		Name: "lease_reclaims_total", Help: "C2D jobs reclaimed after a runner lease expired (crash recovery).",
	})

	federatedJobs = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace", Subsystem: "federated",
		Name: "jobs_total", Help: "Federated jobs by terminal status (released/failed/rejected).",
	}, []string{"status"})

	federatedAggDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "marketplace", Subsystem: "federated",
		Name:    "aggregation_duration_seconds",
		Help:    "Wall-clock time of federated aggregation (FedAvg + store).",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
	})

	federatedParticipants = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace", Subsystem: "federated",
		Name: "participants_total", Help: "Federated participant outcomes (submitted/survived/dropped).",
	}, []string{"role"})
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration, qualityJobs,
		computeJobs, computeJobDuration, computeReclaims,
		federatedJobs, federatedAggDuration, federatedParticipants)
}

// RecordQualityJob counts a completed quality check by its outcome status.
func RecordQualityJob(outcome string) { qualityJobs.WithLabelValues(outcome).Inc() }

// RecordComputeJob counts a C2D job reaching a terminal/transition outcome.
func RecordComputeJob(outcome string) { computeJobs.WithLabelValues(outcome).Inc() }

// ObserveComputeJobDuration records a C2D job's run time by output kind.
func ObserveComputeJobDuration(kind string, seconds float64) {
	if kind == "" {
		kind = "unknown"
	}
	computeJobDuration.WithLabelValues(kind).Observe(seconds)
}

// RecordComputeReclaims counts n jobs reclaimed by the stale-lease sweep.
func RecordComputeReclaims(n int) {
	if n > 0 {
		computeReclaims.Add(float64(n))
	}
}

// RecordFederatedJob counts a federated job reaching a terminal state.
func RecordFederatedJob(status string) { federatedJobs.WithLabelValues(status).Inc() }

// ObserveFederatedAggregation records the wall-clock time of a FedAvg aggregation.
func ObserveFederatedAggregation(seconds float64) { federatedAggDuration.Observe(seconds) }

// RecordFederatedParticipants counts participant outcomes (submitted/survived/dropped).
func RecordFederatedParticipants(role string, n int) {
	if n > 0 {
		federatedParticipants.WithLabelValues(role).Add(float64(n))
	}
}

// Middleware records request count and latency. Place it early in the chain so
// it times the whole handler stack.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched" // 404s etc. — avoid unbounded label values
		}
		httpRequests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
		httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

// Handler serves the Prometheus exposition format (includes go_* / process_*).
func Handler() http.Handler { return promhttp.Handler() }

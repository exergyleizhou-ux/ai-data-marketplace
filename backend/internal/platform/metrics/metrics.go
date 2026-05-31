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
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration)
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

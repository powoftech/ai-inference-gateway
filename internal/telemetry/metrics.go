package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Define Prometheus metrics critical for AI API Gateways
var (
	// ActiveRequests will be used in Phase 5 to trigger the Kubernetes Custom HPA
	ActiveRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_active_requests",
			Help: "Number of currently active streaming connections to the backend",
		},
	)

	TokensGenerated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_tokens_generated_total",
			Help: "Total number of generated tokens tracked for billing and throughput",
		},
		[]string{"model"},
	)

	// TTFT is the paramount AI user experience metric
	TTFTLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_ttft_ms",
			Help:    "Time To First Token (TTFT) in milliseconds",
			Buckets: []float64{50, 100, 200, 300, 500, 1000, 2000},
		},
		[]string{"model"},
	)

	TotalLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_ms",
			Help:    "Total request duration in milliseconds from prompt to final token",
			Buckets: []float64{500, 1000, 2000, 5000, 10000, 30000},
		},
		[]string{"model"},
	)
)

// InitMetrics registers our custom metrics with the default Prometheus registry
func InitMetrics() {
	prometheus.MustRegister(ActiveRequests)
	prometheus.MustRegister(TokensGenerated)
	prometheus.MustRegister(TTFTLatency)
	prometheus.MustRegister(TotalLatency)
}

// MetricsHandler exposes the /metrics endpoint for the Prometheus scraper
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

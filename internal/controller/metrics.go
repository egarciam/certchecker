package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	certificateExpiry = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "certchecker_certificate_expiry_timestamp_seconds",
			Help: "Expiry time of a monitored TLS certificate as a Unix timestamp.",
		},
		[]string{"namespace", "secret"},
	)
	certificatesByStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "certchecker_certificates",
			Help: "Number of monitored certificates by status.",
		},
		[]string{"status"},
	)
	emailsSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "certchecker_emails_sent_total",
			Help: "Number of notification emails sent, by certificate status.",
		},
		[]string{"status"},
	)
	emailsFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "certchecker_emails_failed_total",
			Help: "Number of notification email failures, by certificate status.",
		},
		[]string{"status"},
	)
	reconcileErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "certchecker_reconcile_errors_total",
			Help: "Number of failed reconciliations.",
		},
	)
	reconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "certchecker_reconcile_duration_seconds",
			Help:    "Duration of a reconciliation in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		certificateExpiry,
		certificatesByStatus,
		emailsSent,
		emailsFailed,
		reconcileErrors,
		reconcileDuration,
	)
}

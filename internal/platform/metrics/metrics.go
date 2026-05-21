// Package metrics owns the process-wide Prometheus registry and the metric
// definitions Phase 7 introduced. Other contexts may add their own metrics
// here as the platform's observability story grows; for now the surface is
// limited to the visual-save flow so dashboards can split visual vs
// raw-HTML traffic per spec 014 T116.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is the process-wide collector registry. It is exported so tests can
// reset it between cases and so a non-default health/probe handler can scrape
// from it. We avoid prometheus.DefaultRegisterer because it pulls in the Go
// runtime + process collectors as a side effect; an explicit registry keeps
// the test suite hermetic.
var Registry = prometheus.NewRegistry()

// VisualSavesTotal counts visual-save attempts split by kind (campaign /
// template), result (ok / stale / invalid / forbidden / error), and whether
// the save produced sanitizer-stripped warnings.
//
// Labels:
//   - kind: "campaign" | "template"
//   - result: "ok" | "stale_row" | "invalid_doc" | "unknown_placeholder" |
//     "invalid_media_ref" | "invalid_body" | "forbidden" | "error"
//   - warnings_present: "true" | "false" (only meaningful for result=ok)
var VisualSavesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "nvelope",
		Subsystem: "visual_editor",
		Name:      "saves_total",
		Help:      "Visual-document saves split by kind, outcome, and whether sanitizer warnings were emitted.",
	},
	[]string{"kind", "result", "warnings_present"},
)

// SubscriberFieldMutationsTotal counts mutating subscriber-field registry
// requests by action and outcome. Read traffic is intentionally excluded —
// the registry is small enough that GET volume is not a dashboard signal.
//
// Labels:
//   - action: "create" | "update" | "delete" | "reorder"
//   - result: "ok" | "conflict" | "invalid" | "forbidden" | "error"
var SubscriberFieldMutationsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "nvelope",
		Subsystem: "subscriber_fields",
		Name:      "mutations_total",
		Help:      "Mutating requests on the tenant subscriber-fields registry, split by action and outcome.",
	},
	[]string{"action", "result"},
)

func init() {
	Registry.MustRegister(VisualSavesTotal)
	Registry.MustRegister(SubscriberFieldMutationsTotal)
}

// Handler returns an http.Handler that serves the registry's metrics in the
// Prometheus exposition format. Mount it on /metrics in the api command.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

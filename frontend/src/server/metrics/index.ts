// BFF-side Prometheus instrumentation. The Go API owns the visual-save
// counter (kind × result × warnings_present); the BFF adds the
// render-latency histogram for `renderVisualDoc` so dashboards can split
// total visual-save latency into "BFF render" vs "Go validate + persist".
//
// The exporter is mounted by the Nitro /metrics route shim
// (frontend/src/server/routes/metrics.get.ts) so a single scrape target
// returns process-level + custom metrics.

import {
  Counter,
  Histogram,
  Registry,
  collectDefaultMetrics,
} from "prom-client"

// registry is the BFF's local registry — same rationale as the Go side:
// we avoid the global default registry so test runs stay hermetic and
// the metric set is observable from one place.
export const registry = new Registry()

collectDefaultMetrics({ register: registry })

// renderDurationSeconds captures wall-clock latency of `renderVisualDoc`,
// labelled by what the BFF is rendering for (the campaign or template save
// path, or a render-preview call). Future log/alert pipelines can derive
// percentiles per surface without re-instrumentation.
//
// Labels:
//   - surface: "campaign_save" | "template_save" | "preview"
//   - result: "ok" | "error"
export const renderDurationSeconds = new Histogram({
  name: "nvelope_bff_render_duration_seconds",
  help: "Latency of @react-email visual-doc rendering, split by surface and outcome.",
  labelNames: ["surface", "result"] as const,
  // Buckets tuned for transactional email rendering: most docs render in
  // under 200 ms; 5 s is well past the BFF's hard timeout, so anything
  // higher is logged as +Inf and means something is wrong.
  buckets: [0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5],
  registers: [registry],
})

// observeRender wraps an async render call and records its duration. The
// returned value is the wrapped fn's resolved value; thrown errors land
// in the "error" bucket and are re-thrown so the caller's existing error
// handling runs unchanged.
export async function observeRender<T>(
  surface: "campaign_save" | "template_save" | "preview",
  fn: () => Promise<T>,
): Promise<T> {
  const end = renderDurationSeconds.startTimer({ surface })
  try {
    const out = await fn()
    end({ result: "ok" })
    return out
  } catch (err) {
    end({ result: "error" })
    throw err
  }
}

// bffSaveAttemptsTotal mirrors the Go-side VisualSavesTotal but counts
// what the BFF saw — so a 502_bad_gateway here vs an ok on the Go side
// helps narrow down which tier dropped the request.
//
// Labels:
//   - surface: "campaign_save" | "template_save" | "preview"
//   - kind: "ok" | "validation_error" | "bad_gateway" | "go_error"
export const bffSaveAttemptsTotal = new Counter({
  name: "nvelope_bff_save_attempts_total",
  help: "Visual-save / preview attempts as seen by the BFF, by surface and outcome kind.",
  labelNames: ["surface", "kind"] as const,
  registers: [registry],
})

// metricsResponse returns the registry's exposition-format payload + the
// content-type header. The Nitro route shim uses it directly.
export async function metricsResponse(): Promise<{
  body: string
  contentType: string
}> {
  return {
    body: await registry.metrics(),
    contentType: registry.contentType,
  }
}

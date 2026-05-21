// Nitro file-based-routing shim for GET /metrics. Exposes the BFF-side
// Prometheus registry (default process collectors + visual-editor render
// latency + per-surface save attempts). Mounted at the application root,
// not under /t/:slug, because the metrics are tenant-agnostic.

import { defineHandler } from "nitro"
import { metricsResponse } from "@/server/metrics"

export default defineHandler(async (event) => {
  const { body, contentType } = await metricsResponse()
  event.res.headers.set("content-type", contentType)
  return body
})

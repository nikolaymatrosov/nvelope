// Nitro file-based-routing shim for PUT /t/:slug/api/templates/:id/visual.
// Placeholder: returns 501 Not Implemented until T072/T077 land the
// templates-side orchestrator. The path is reserved here so the vite-proxy
// rewrite (T050) can already point at Nitro for templates as well as
// campaigns, keeping the proxy patterns identical across the two editors.

import { defineHandler } from "nitro"

export default defineHandler((event) => {
  event.res.status = 501
  return {
    error: "not_implemented",
    message:
      "templates visual save lands in T072/T077 — the BFF reserves this path so " +
      "the vite proxy can route templates and campaigns to Nitro identically.",
  }
})

// Nitro file-based-routing shim for POST /t/:slug/api/render-preview.
// Reads the H3Event, delegates to the testable orchestrator in
// ../../../render-preview.ts, maps the result back to an h3 response.
//
// Tenant-scoped, never row-scoped — the body carries the unsaved bodyDoc
// directly. Shared by both the campaign and template editors (per the
// 2026-05-20 N4 clarification).

import { defineHandler } from "nitro"
import { runRenderPreview } from "../../../render-preview"
import type {
  RenderPreviewSample,
} from "../../../render-preview"
import type { Theme, VisualDoc } from "../../../../render/types"

type IncomingBody = {
  bodyDoc?: unknown
  theme?: unknown
  sample?: unknown
}

export default defineHandler(async (event) => {
  const slug = event.context.params?.slug
  if (!slug) {
    event.res.status = 400
    return { error: "invalid_path", message: "tenant slug is required" }
  }
  const raw = (await event.req.json()) as IncomingBody
  if (!raw.bodyDoc) {
    event.res.status = 400
    return { error: "invalid_body", message: "bodyDoc is required" }
  }

  const cookie = event.req.headers.get("cookie") ?? ""
  const requestId = event.req.headers.get("x-request-id") ?? crypto.randomUUID()

  const result = await runRenderPreview({
    slug,
    cookie,
    requestId,
    goApiBaseUrl: process.env.NV_GO_API_URL,
    mediaUrlPrefix: process.env.OBJECT_STORAGE_PUBLIC_BASE_URL ?? "",
    body: {
      bodyDoc: raw.bodyDoc as VisualDoc,
      theme: raw.theme as Theme | null,
      sample: raw.sample as RenderPreviewSample | undefined,
    },
  })
  event.res.status = result.status
  return result.body
})

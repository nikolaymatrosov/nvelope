// Nitro file-based-routing shim for PUT /t/:slug/api/templates/:id/visual.
// Reads the H3Event, delegates to the testable orchestrator in
// ../../../../../visual-save.ts (`runVisualTemplateSave`), maps the result
// back to an h3 response. Mirrors the campaigns shim under
// ../../campaigns/[id]/visual.put.ts — same shape, different Go endpoint.

import { defineHandler } from "nitro"
import type { Theme, VisualDoc } from "@/server/render/types"
import { runVisualTemplateSave } from "@/server/routes/visual-save"

type IncomingBody = {
  name?: unknown
  kind?: unknown
  subject?: unknown
  bodyDoc?: unknown
  theme?: unknown
  ifUnmodifiedSince?: unknown
}

export default defineHandler(async (event) => {
  const slug = event.context.params?.slug
  const id = event.context.params?.id
  if (!slug || !id) {
    event.res.status = 400
    return { error: "invalid_path", message: "tenant slug and template id are required" }
  }
  const raw = (await event.req.json()) as IncomingBody
  if (
    typeof raw.name !== "string" ||
    typeof raw.subject !== "string" ||
    typeof raw.ifUnmodifiedSince !== "string" ||
    !raw.bodyDoc ||
    (raw.kind !== "campaign" && raw.kind !== "transactional")
  ) {
    event.res.status = 400
    return {
      error: "invalid_body",
      message: "name, kind, subject, bodyDoc, and ifUnmodifiedSince are required",
    }
  }

  const cookie = event.req.headers.get("cookie") ?? ""
  const requestId = event.req.headers.get("x-request-id") ?? crypto.randomUUID()

  const result = await runVisualTemplateSave({
    slug,
    templateId: id,
    cookie,
    requestId,
    goApiBaseUrl: process.env.NV_GO_API_URL,
    mediaUrlPrefix: process.env.OBJECT_STORAGE_PUBLIC_BASE_URL ?? "",
    body: {
      name: raw.name,
      kind: raw.kind,
      subject: raw.subject,
      bodyDoc: raw.bodyDoc as VisualDoc,
      theme: raw.theme as Theme | null,
      ifUnmodifiedSince: raw.ifUnmodifiedSince,
    },
  })
  event.res.status = result.status
  return result.body
})

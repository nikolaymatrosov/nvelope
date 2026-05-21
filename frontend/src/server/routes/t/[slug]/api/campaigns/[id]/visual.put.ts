// Nitro file-based-routing shim for PUT /t/:slug/api/campaigns/:id/visual.
// Reads the H3Event, delegates to the testable orchestrator in
// ../../../../../visual-save.ts, maps the result back to an h3 response.

import { defineHandler } from "nitro"
import { runVisualCampaignSave } from "../../../../../visual-save"
import type { Theme, VisualDoc } from "../../../../../../render/types"

type IncomingBody = {
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
    return { error: "invalid_path", message: "tenant slug and campaign id are required" }
  }
  const raw = (await event.req.json()) as IncomingBody
  if (
    typeof raw.subject !== "string" ||
    typeof raw.ifUnmodifiedSince !== "string" ||
    !raw.bodyDoc
  ) {
    event.res.status = 400
    return { error: "invalid_body", message: "subject, bodyDoc, and ifUnmodifiedSince are required" }
  }

  const cookie = event.req.headers.get("cookie") ?? ""
  const requestId = event.req.headers.get("x-request-id") ?? crypto.randomUUID()

  const result = await runVisualCampaignSave({
    slug,
    campaignId: id,
    cookie,
    requestId,
    goApiBaseUrl: process.env.NV_GO_API_URL,
    mediaUrlPrefix: process.env.OBJECT_STORAGE_PUBLIC_BASE_URL ?? "",
    body: {
      subject: raw.subject,
      bodyDoc: raw.bodyDoc as VisualDoc,
      theme: raw.theme as Theme | null,
      ifUnmodifiedSince: raw.ifUnmodifiedSince,
    },
  })
  event.res.status = result.status
  return result.body
})

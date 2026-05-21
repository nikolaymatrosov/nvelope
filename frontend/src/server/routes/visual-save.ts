// Pure orchestration for the visual-save Nitro route. The actual h3 file-
// based-routing shims (e.g. ./t/[slug]/api/campaigns/[id]/visual.put.ts)
// parse the H3Event and delegate here so the orchestrator stays testable
// without an H3 runtime.
//
// Flow per specs/014-visual-email-editor/contracts/tenant-api.md and the
// 2026-05-20 fail-closed clarification:
//   1. Fetch the tenant's subscriber-field registry from Go.
//   2. Validate the doc against the registry + the static rules.
//   3. If the operator did not pin a theme, fetch branding from Go and
//      resolve the effective theme via the platform defaults.
//   4. Render the doc to HTML + plain text with @react-email.
//   5. Forward { bodyDoc, bodyHtml, bodyText, theme, ifUnmodifiedSince } to
//      Go's PUT /campaigns/{id}/visual.
//   6. Return Go's response verbatim — including its status code.
//
// Any side-call failure to Go surfaces as `502 bad_gateway` (fail-closed).
// Validation errors return 400 with the typed error kind. Stale-row 409s
// and forbidden 403s from Go pass through unchanged.

import {
  
  GoApiError,
  GoApiUnreachable,
  
  
  createGoApiClient
} from "../clients/go-api"
import { PlatformDefaultTheme, renderVisualDoc } from "../render"
import { ValidatorError, validateVisualDoc } from "../validate"
import type {BrandingResponse, PutVisualPayload, PutVisualResponse} from "../clients/go-api";
import type { Theme, VisualDoc } from "../render/types"

export type VisualSaveInput = {
  slug: string
  campaignId: string
  cookie: string
  requestId: string
  goApiBaseUrl?: string
  mediaUrlPrefix: string
  body: {
    subject: string
    bodyDoc: VisualDoc
    theme: Theme | null
    ifUnmodifiedSince: string
  }
  // log is the BFF's structured logger; the route shim wires it to whatever
  // Nitro's logger or a plain console wrapper. Required so requests
  // correlate across BFF and Go via tenant_id, actor_id, request_id (per
  // plan.md Principle V).
  log?: (
    level: "info" | "warn" | "error",
    event: string,
    fields: Record<string, unknown>,
  ) => void
  // fetchImpl is injected for tests (msw mocks). Defaults to global fetch.
  fetchImpl?: typeof fetch
}

export type VisualSaveResult =
  | { kind: "ok"; status: number; body: PutVisualResponse }
  | { kind: "validation_error"; status: 400; body: { error: string; kind: string; placeholders?: Array<string> } }
  | { kind: "bad_gateway"; status: 502; body: { error: "bad_gateway"; detail: string } }
  | { kind: "go_error"; status: number; body: unknown }

export async function runVisualCampaignSave(input: VisualSaveInput): Promise<VisualSaveResult> {
  const client = createGoApiClient({
    baseUrl: input.goApiBaseUrl,
    requestId: input.requestId,
    fetchImpl: input.fetchImpl,
  })

  const logFields = {
    tenant_slug: input.slug,
    campaign_id: input.campaignId,
    request_id: input.requestId,
  }

  let fields: { fields: Array<{ slug: string }> }
  try {
    fields = await client.listSubscriberFields(input.cookie, input.slug)
  } catch (err) {
    return badGateway(err, "fetching subscriber fields", logFields, input.log)
  }

  const knownSlugs = new Set(fields.fields.map((f) => f.slug))
  try {
    validateVisualDoc(input.body.bodyDoc, {
      knownSlugs,
      mediaUrlPrefix: input.mediaUrlPrefix,
    })
  } catch (err) {
    if (err instanceof ValidatorError) {
      return {
        kind: "validation_error",
        status: 400,
        body: {
          error: err.kind,
          kind: err.kind,
          ...(err.placeholders.length > 0 ? { placeholders: err.placeholders } : {}),
        },
      }
    }
    throw err
  }

  let effectiveTheme: Theme
  if (input.body.theme) {
    effectiveTheme = input.body.theme
  } else {
    let branding: BrandingResponse
    try {
      branding = await client.getBranding(input.cookie, input.slug)
    } catch (err) {
      return badGateway(err, "fetching branding", logFields, input.log)
    }
    effectiveTheme = themeFromBranding(branding)
  }

  const { bodyHtml, bodyText } = await renderVisualDoc(input.body.bodyDoc, effectiveTheme)

  const payload: PutVisualPayload = {
    subject: input.body.subject,
    bodyDoc: input.body.bodyDoc,
    bodyHtml,
    bodyText,
    // Persist null when the operator did not pin a theme so future branding
    // changes propagate on next save (per FR-022 / plan.md US3).
    theme: input.body.theme,
    ifUnmodifiedSince: input.body.ifUnmodifiedSince,
  }

  try {
    const goRes = await client.putCampaignVisual(input.cookie, input.slug, input.campaignId, payload)
    input.log?.("info", "visual_save.ok", { ...logFields, warnings_count: goRes.warnings.length })
    return { kind: "ok", status: 200, body: goRes }
  } catch (err) {
    if (err instanceof GoApiError) {
      input.log?.("warn", "visual_save.go_error", { ...logFields, status: err.status })
      return { kind: "go_error", status: err.status, body: err.body }
    }
    return badGateway(err, "forwarding save to Go", logFields, input.log)
  }
}

// themeFromBranding mirrors Go's `Theme.DefaultsFromBranding` (Phase 6 →
// Phase 7 hand-off). Branding fields the BFF doesn't recognize fall back to
// the platform default so a partial branding row still renders.
export function themeFromBranding(b: BrandingResponse): Theme {
  return {
    textColor: b.text_color || PlatformDefaultTheme.textColor,
    linkColor: b.primary_color || PlatformDefaultTheme.linkColor,
    buttonColor: b.primary_color || PlatformDefaultTheme.buttonColor,
    buttonTextColor: PlatformDefaultTheme.buttonTextColor,
    fontFamily: b.font_family || PlatformDefaultTheme.fontFamily,
    containerWidth: PlatformDefaultTheme.containerWidth,
  }
}

function badGateway(
  err: unknown,
  detail: string,
  logFields: Record<string, unknown>,
  log: VisualSaveInput["log"],
): VisualSaveResult {
  const message = err instanceof GoApiUnreachable ? err.message : String(err)
  log?.("error", "visual_save.bad_gateway", { ...logFields, detail, error: message })
  return {
    kind: "bad_gateway",
    status: 502,
    body: { error: "bad_gateway", detail },
  }
}

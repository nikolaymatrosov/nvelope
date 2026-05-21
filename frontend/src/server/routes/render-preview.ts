// Pure orchestration for the render-preview Nitro route. Tenant-scoped
// (shared by campaign and template editors per the 2026-05-20 N4
// clarification) — never reads a row, only the supplied bodyDoc.
//
// Flow per specs/014-visual-email-editor/contracts/tenant-api.md:
//   1. Fetch the tenant's subscriber-field registry from Go.
//   2. Validate the supplied doc against the registry + static rules.
//   3. If theme is null, fetch branding from Go and resolve defaults.
//   4. Render the doc to HTML + plain text with @react-email.
//   5. If `sample` was supplied, side-call Go's POST /substitute-sample to
//      resolve {{ subscriber.* }} / {{ campaign.* }} placeholders through
//      the canonical send-pipeline substituter. BFF MUST NOT reimplement
//      substitution rules in TS (per research.md § R12b).
//   6. Run the preview-output sanitizer (FR-014a) over the resulting HTML.
//   7. Return { bodyHtml, bodyText, warnings }.
//
// Never persists. Any side-call failure to Go surfaces as 502 bad_gateway
// (fail-closed). Validation errors return 400 with the typed error kind.

import {

  GoApiUnreachable,
  createGoApiClient
} from "../clients/go-api"
import { bffSaveAttemptsTotal, observeRender } from "../metrics"
import { PlatformDefaultTheme, renderVisualDoc, sanitizePreviewHtml } from "../render"
import { ValidatorError, validateVisualDoc } from "../validate"
import { themeFromBranding } from "./visual-save"
import type {BrandingResponse} from "../clients/go-api";
import type { RenderWarning, Theme, VisualDoc } from "../render/types"

export type RenderPreviewSample = {
  subscriber: Record<string, unknown>
  campaign: Record<string, unknown>
}

export type RenderPreviewInput = {
  slug: string
  cookie: string
  requestId: string
  goApiBaseUrl?: string
  mediaUrlPrefix: string
  body: {
    bodyDoc: VisualDoc
    theme: Theme | null
    sample?: RenderPreviewSample
  }
  log?: (
    level: "info" | "warn" | "error",
    event: string,
    fields: Record<string, unknown>,
  ) => void
  fetchImpl?: typeof fetch
}

export type RenderPreviewResult =
  | {
      kind: "ok"
      status: 200
      body: { bodyHtml: string; bodyText: string; warnings: Array<RenderWarning> }
    }
  | {
      kind: "validation_error"
      status: 400
      body: { error: string; kind: string; placeholders?: Array<string> }
    }
  | { kind: "bad_gateway"; status: 502; body: { error: "bad_gateway"; detail: string } }

export async function runRenderPreview(input: RenderPreviewInput): Promise<RenderPreviewResult> {
  const client = createGoApiClient({
    baseUrl: input.goApiBaseUrl,
    requestId: input.requestId,
    fetchImpl: input.fetchImpl,
  })

  const log = resolveLog(input.log)
  const logFields = { tenant_slug: input.slug, request_id: input.requestId }

  let knownSlugs: Set<string>
  try {
    const fields = await client.listSubscriberFields(input.cookie, input.slug)
    knownSlugs = new Set(fields.fields.map((f) => f.slug))
  } catch (err) {
    return badGateway(err, "fetching subscriber fields", logFields, log)
  }

  try {
    validateVisualDoc(input.body.bodyDoc, {
      knownSlugs,
      mediaUrlPrefix: input.mediaUrlPrefix,
    })
  } catch (err) {
    if (err instanceof ValidatorError) {
      bffSaveAttemptsTotal.inc({ surface: "preview", kind: "validation_error" })
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
      return badGateway(err, "fetching branding", logFields, log)
    }
    effectiveTheme = themeFromBranding(branding)
  }

  const renderStart = Date.now()
  const rendered = await observeRender("preview", () =>
    renderVisualDoc(input.body.bodyDoc, effectiveTheme),
  )
  const renderDurationMs = Date.now() - renderStart
  let { bodyHtml, bodyText } = rendered
  const warnings = [...rendered.warnings]

  if (input.body.sample) {
    try {
      const substituted = await client.substituteSample(input.cookie, input.slug, {
        html: bodyHtml,
        text: bodyText,
        sample: input.body.sample,
      })
      bodyHtml = substituted.html
      bodyText = substituted.text
    } catch (err) {
      return badGateway(err, "substituting sample placeholders", logFields, log)
    }
  }

  const sanitized = sanitizePreviewHtml(bodyHtml)
  bodyHtml = sanitized.html
  warnings.push(...sanitized.warnings)

  bffSaveAttemptsTotal.inc({ surface: "preview", kind: "ok" })
  log("info", "render_preview.ok", {
    ...logFields,
    warnings_count: warnings.length,
    render_duration_ms: renderDurationMs,
  })
  return {
    kind: "ok",
    status: 200,
    body: { bodyHtml, bodyText, warnings },
  }
}

function badGateway(
  err: unknown,
  detail: string,
  logFields: Record<string, unknown>,
  log: NonNullable<RenderPreviewInput["log"]>,
): RenderPreviewResult {
  const message = err instanceof GoApiUnreachable ? err.message : String(err)
  bffSaveAttemptsTotal.inc({ surface: "preview", kind: "bad_gateway" })
  log("error", "render_preview.bad_gateway", { ...logFields, detail, error: message })
  return {
    kind: "bad_gateway",
    status: 502,
    body: { error: "bad_gateway", detail },
  }
}

// resolveLog returns the supplied logger or a console-backed NDJSON shim
// matching Go's slog handler shape — single log pipeline ingests both.
function resolveLog(
  log: RenderPreviewInput["log"],
): NonNullable<RenderPreviewInput["log"]> {
  if (log) return log
  return (level, event, fields) => {
    const line = JSON.stringify({
      level,
      service: "bff",
      time: new Date().toISOString(),
      event,
      ...fields,
    })
    if (level === "error") {
      console.error(line)
    } else if (level === "warn") {
      console.warn(line)
    } else {
      console.log(line)
    }
  }
}

// Re-export for the route shims so they can use PlatformDefaultTheme as a
// last-ditch fallback if branding is misconfigured in dev.
export { PlatformDefaultTheme }

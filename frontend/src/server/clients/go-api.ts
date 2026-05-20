// Typed Go-API client for the Nitro server tier (visual-save,
// render-preview). The BFF runs server-side, so cookies and X-Request-Id
// must be forwarded explicitly — there is no implicit `credentials: include`
// the way the browser fetch wrapper has it.
//
// All side-call responses bubble Go's response codes verbatim: a 403 from
// Go becomes a 403 from the BFF, a 409 stale_row from Go's visual-save tail
// becomes a 409 from the BFF route. Network/transport failures surface as
// GoApiUnreachable so the route can fail closed with 502 bad_gateway per
// the 2026-05-20 fail-closed clarification.

import type { RenderWarning, Theme, VisualDoc } from "../render/types"

// GoApiUnreachable is thrown when the BFF cannot reach the Go API at all
// (DNS failure, connection refused, request timeout). The Nitro routes
// catch this and emit 502 bad_gateway to the SPA — the fail-closed
// semantics from the 2026-05-20 spec clarification.
export class GoApiUnreachable extends Error {
  readonly cause?: unknown
  constructor(message: string, cause?: unknown) {
    super(message)
    this.cause = cause
  }
}

// GoApiError carries a non-2xx response verbatim so the route can forward
// the status, slug, and any extra fields (e.g. `currentUpdatedAt` from the
// 409 stale_row payload) to the SPA unchanged.
export class GoApiError extends Error {
  readonly status: number
  readonly body: unknown
  constructor(status: number, body: unknown) {
    const slug =
      typeof body === "object" && body !== null && "error" in body
        ? String((body).error)
        : `http_${status}`
    super(`Go API responded ${status}: ${slug}`)
    this.status = status
    this.body = body
  }
}

// ── Wire-shape types matching the contracts in
//    specs/014-visual-email-editor/contracts/tenant-api.md.

export type SubscriberFieldView = {
  id: string
  slug: string
  displayName: string
  type: string
  defaultValue: string
  position: number
  builtIn: boolean
  createdAt: string
  updatedAt: string
}

export type ListSubscriberFieldsResponse = {
  fields: Array<SubscriberFieldView>
}

export type BrandingResponse = {
  primary_color: string
  text_color: string
  background_color: string
  font_family: string
  logo_url: string | null
  // The full Phase 6 branding shape is wider than this; the BFF only needs
  // the colors + font that feed Theme.DefaultsFromBranding.
}

export type PutVisualPayload = {
  subject: string
  bodyDoc: VisualDoc
  bodyHtml: string
  bodyText: string
  theme: Theme | null
  ifUnmodifiedSince: string
}

export type PutVisualTemplatePayload = PutVisualPayload & {
  name: string
  kind: "campaign" | "transactional"
}

export type PutVisualResponse = {
  campaign?: unknown
  template?: unknown
  warnings: Array<RenderWarning>
  updatedAt: string
}

export type SubstituteSamplePayload = {
  html: string
  text: string
  sample: {
    subscriber: Record<string, unknown>
    campaign: Record<string, unknown>
  }
}

export type SubstituteSampleResponse = {
  html: string
  text: string
}

// ── Client ────────────────────────────────────────────────────────────────

export type GoApiClient = {
  listSubscriberFields: (cookie: string, slug: string) => Promise<ListSubscriberFieldsResponse>
  getBranding: (cookie: string, slug: string) => Promise<BrandingResponse>
  putCampaignVisual: (
    cookie: string,
    slug: string,
    id: string,
    payload: PutVisualPayload,
  ) => Promise<PutVisualResponse>
  putTemplateVisual: (
    cookie: string,
    slug: string,
    id: string,
    payload: PutVisualTemplatePayload,
  ) => Promise<PutVisualResponse>
  substituteSample: (
    cookie: string,
    slug: string,
    payload: SubstituteSamplePayload,
  ) => Promise<SubstituteSampleResponse>
}

export type CreateGoApiClientOptions = {
  // baseUrl is the Go API origin — read from
  // process.env.NV_GO_API_URL in production composition and overridable
  // for tests. Defaults to localhost:8080 to match the dev Vite proxy.
  baseUrl?: string
  // requestId is propagated to Go via the X-Request-Id header so a single
  // user trace correlates across BFF and Go logs (per plan.md Principle V).
  requestId: string
  fetchImpl?: typeof fetch
}

export function createGoApiClient(opts: CreateGoApiClientOptions): GoApiClient {
  const baseUrl = opts.baseUrl ?? "http://localhost:8080"
  const fetchImpl = opts.fetchImpl ?? fetch

  function url(slug: string, suffix: string): string {
    return `${baseUrl}/t/${encodeURIComponent(slug)}/api${suffix}`
  }

  async function send(
    method: string,
    fullUrl: string,
    cookie: string,
    body?: unknown,
  ): Promise<unknown> {
    let res: Response
    try {
      res = await fetchImpl(fullUrl, {
        method,
        headers: {
          ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
          ...(cookie ? { Cookie: cookie } : {}),
          "X-Request-Id": opts.requestId,
        },
        body: body !== undefined ? JSON.stringify(body) : undefined,
      })
    } catch (err) {
      throw new GoApiUnreachable(`Go API unreachable: ${method} ${fullUrl}`, err)
    }
    let parsed: unknown = null
    const text = await res.text()
    if (text) {
      try {
        parsed = JSON.parse(text)
      } catch {
        parsed = text
      }
    }
    if (!res.ok) {
      throw new GoApiError(res.status, parsed)
    }
    return parsed
  }

  return {
    listSubscriberFields(cookie, slug) {
      return send("GET", url(slug, "/subscriber-fields"), cookie) as Promise<
        ListSubscriberFieldsResponse
      >
    },
    getBranding(cookie, slug) {
      return send("GET", url(slug, "/branding"), cookie) as Promise<BrandingResponse>
    },
    putCampaignVisual(cookie, slug, id, payload) {
      return send(
        "PUT",
        url(slug, `/campaigns/${encodeURIComponent(id)}/visual`),
        cookie,
        payload,
      ) as Promise<PutVisualResponse>
    },
    putTemplateVisual(cookie, slug, id, payload) {
      return send(
        "PUT",
        url(slug, `/templates/${encodeURIComponent(id)}/visual`),
        cookie,
        payload,
      ) as Promise<PutVisualResponse>
    },
    substituteSample(cookie, slug, payload) {
      return send(
        "POST",
        url(slug, "/substitute-sample"),
        cookie,
        payload,
      ) as Promise<SubstituteSampleResponse>
    },
  }
}

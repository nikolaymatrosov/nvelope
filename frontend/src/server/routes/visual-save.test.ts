import { describe, expect, it, vi } from "vitest"
import { runVisualCampaignSave } from "./visual-save"
import type { VisualDoc } from "../render/types"

// Each fake response handler returns a `Response` for one specific
// (method, path) tuple — calls that don't match an expected route fail
// the test immediately, so each test is explicit about which Go endpoints
// it expects the orchestrator to hit.
type FakeRoute = (
  url: string,
  init: RequestInit,
) => Promise<Response> | Response | undefined
function fakeFetch(routes: Array<FakeRoute>): {
  fetch: typeof fetch
  calls: Array<{ url: string; method: string; headers: Headers }>
} {
  const calls: Array<{ url: string; method: string; headers: Headers }> = []
  const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const i = init ?? {}
    calls.push({
      url,
      method: i.method ?? "GET",
      headers: new Headers(i.headers),
    })
    for (const r of routes) {
      const out = await r(url, i)
      if (out) return out
    }
    throw new Error(`unexpected fetch: ${i.method ?? "GET"} ${url}`)
  })
  return { fetch: impl, calls }
}

function on(
  method: string,
  pathSuffix: string,
  respond: (init: RequestInit) => Response | Promise<Response>,
): FakeRoute {
  return (url, init) => {
    if (url.endsWith(pathSuffix) && (init.method ?? "GET") === method) {
      return respond(init)
    }
    return undefined
  }
}

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })

const subscriberFieldsOk = on("GET", "/subscriber-fields", () =>
  json({ fields: [{ slug: "first_name" }, { slug: "email" }] }),
)
const brandingOk = on("GET", "/branding", () =>
  json({
    primary_color: "#5566aa",
    text_color: "#111111",
    background_color: "#ffffff",
    font_family: "Inter, sans-serif",
    logo_url: null,
  }),
)
const visualSaveOk = on("PUT", "/visual", () =>
  json({
    campaign: { id: "c-1" },
    warnings: [],
    updatedAt: "2026-05-21T00:00:00Z",
  }),
)

function makeBody(doc: VisualDoc): {
  subject: string
  bodyDoc: VisualDoc
  theme: null
  ifUnmodifiedSince: string
} {
  return {
    subject: "Subject",
    bodyDoc: doc,
    theme: null,
    ifUnmodifiedSince: "2026-05-20T12:34:56Z",
  }
}

const minimalDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }],
}

describe("runVisualCampaignSave", () => {
  it("happy path: forwards rendered html/text + cookie + X-Request-Id to Go", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk, visualSaveOk])
    const out = await runVisualCampaignSave({
      slug: "acme",
      campaignId: "c-1",
      cookie: "nv_workspace=abc",
      requestId: "req-xyz",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: makeBody(minimalDoc),
      fetchImpl: ff.fetch,
    })

    expect(out.kind).toBe("ok")
    expect(out.status).toBe(200)

    // Cookie + X-Request-Id forwarded on every call (3 calls: fields,
    // branding, save).
    expect(ff.calls).toHaveLength(3)
    for (const c of ff.calls) {
      expect(c.headers.get("Cookie")).toBe("nv_workspace=abc")
      expect(c.headers.get("X-Request-Id")).toBe("req-xyz")
    }
    expect(ff.calls.map((c) => `${c.method} ${urlPath(c.url)}`)).toEqual([
      "GET /t/acme/api/subscriber-fields",
      "GET /t/acme/api/branding",
      "PUT /t/acme/api/campaigns/c-1/visual",
    ])
  })

  it("does not fetch branding when the caller pins a theme", async () => {
    const ff = fakeFetch([subscriberFieldsOk, visualSaveOk])
    const out = await runVisualCampaignSave({
      slug: "acme",
      campaignId: "c-1",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: {
        ...makeBody(minimalDoc),
        theme: {
          textColor: "#000",
          linkColor: "#00f",
          buttonColor: "#00f",
          buttonTextColor: "#fff",
          fontFamily: "Arial",
          containerWidth: 600,
        },
      },
      fetchImpl: ff.fetch,
    })
    expect(out.kind).toBe("ok")
    expect(ff.calls.map((c) => urlPath(c.url))).not.toContain("/t/acme/api/branding")
  })

  it("fails closed with 502 bad_gateway when subscriber-fields fetch fails", async () => {
    const ff = fakeFetch([
      on("GET", "/subscriber-fields", () => json({ error: "boom" }, 500)),
    ])
    const out = await runVisualCampaignSave({
      slug: "acme",
      campaignId: "c-1",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: makeBody(minimalDoc),
      fetchImpl: ff.fetch,
    })
    // listSubscriberFields surfaces a 5xx as GoApiError; the orchestrator
    // collapses it to 502 because the BFF cannot continue without the
    // field list (fail-closed per the 2026-05-20 clarification).
    expect(out.status).toBe(502)
    expect(out.kind).toBe("bad_gateway")
  })

  it("fails closed with 502 when Go is unreachable for branding", async () => {
    const ff = fakeFetch([
      subscriberFieldsOk,
      on("GET", "/branding", () => Promise.reject(new Error("ECONNREFUSED"))),
    ])
    const out = await runVisualCampaignSave({
      slug: "acme",
      campaignId: "c-1",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: makeBody(minimalDoc),
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(502)
    expect(out.kind).toBe("bad_gateway")
  })

  it("rejects an unknown subscriber slug before reaching Go's save endpoint", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk])
    const out = await runVisualCampaignSave({
      slug: "acme",
      campaignId: "c-1",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: makeBody({
        version: 1,
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [
              { type: "mergeTag", attrs: { namespace: "subscriber", key: "unknown_field" } },
            ],
          },
        ],
      }),
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(400)
    expect(out.kind).toBe("validation_error")
    if (out.kind === "validation_error") {
      expect(out.body.kind).toBe("unknown_placeholder")
      expect(out.body.placeholders).toEqual(["subscriber.unknown_field"])
    }
    // PUT was never sent — validation happens before render.
    expect(ff.calls.map((c) => c.method)).not.toContain("PUT")
  })

  it("forwards a 409 stale_row from Go to the SPA verbatim", async () => {
    const ff = fakeFetch([
      subscriberFieldsOk,
      brandingOk,
      on("PUT", "/visual", () =>
        json(
          {
            error: "stale_row",
            kind: "stale_row",
            currentUpdatedAt: "2026-05-21T00:30:00Z",
          },
          409,
        ),
      ),
    ])
    const out = await runVisualCampaignSave({
      slug: "acme",
      campaignId: "c-1",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: makeBody(minimalDoc),
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(409)
    expect(out.kind).toBe("go_error")
    if (out.kind === "go_error") {
      const body = out.body as { kind: string; currentUpdatedAt: string }
      expect(body.kind).toBe("stale_row")
      expect(body.currentUpdatedAt).toBe("2026-05-21T00:30:00Z")
    }
  })
})

function urlPath(u: string): string {
  return new URL(u).pathname
}

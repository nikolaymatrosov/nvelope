import { describe, expect, it, vi } from "vitest"
import { runVisualTemplateSave } from "./visual-save"
import type { VisualDoc } from "../render/types"

// Same harness shape as visual-save.test.ts. Each fake response handler
// returns a `Response` for one specific (method, path) tuple; calls that
// don't match an expected route fail the test immediately.
type FakeRoute = (
  url: string,
  init: RequestInit,
) => Promise<Response> | Response | undefined
function fakeFetch(routes: Array<FakeRoute>): {
  fetch: typeof fetch
  calls: Array<{ url: string; method: string; headers: Headers; body: string }>
} {
  const calls: Array<{ url: string; method: string; headers: Headers; body: string }> = []
  const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const i = init ?? {}
    calls.push({
      url,
      method: i.method ?? "GET",
      headers: new Headers(i.headers),
      body: typeof i.body === "string" ? i.body : "",
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
const templateSaveOk = on("PUT", "/visual", () =>
  json({
    template: { id: "t-1" },
    warnings: [],
    updatedAt: "2026-05-21T00:00:00Z",
  }),
)

const minimalDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }],
}

function makeBody(doc: VisualDoc): {
  name: string
  kind: "campaign"
  subject: string
  bodyDoc: VisualDoc
  theme: null
  ifUnmodifiedSince: string
} {
  return {
    name: "Welcome series — week 1",
    kind: "campaign",
    subject: "Subject",
    bodyDoc: doc,
    theme: null,
    ifUnmodifiedSince: "2026-05-20T12:34:56Z",
  }
}

describe("runVisualTemplateSave", () => {
  it("happy path: forwards rendered html/text + cookie + X-Request-Id to Go's templates endpoint", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk, templateSaveOk])
    const out = await runVisualTemplateSave({
      slug: "acme",
      templateId: "t-1",
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
      "PUT /t/acme/api/templates/t-1/visual",
    ])

    // The PUT body includes name + kind so Go's handler can audit the
    // template type and the BFF-supplied bodyHtml + bodyText.
    const putCall = ff.calls.find((c) => c.method === "PUT")!
    const body = JSON.parse(putCall.body) as Record<string, unknown>
    expect(body.name).toBe("Welcome series — week 1")
    expect(body.kind).toBe("campaign")
    expect(body.subject).toBe("Subject")
    expect(typeof body.bodyHtml).toBe("string")
    expect(typeof body.bodyText).toBe("string")
    expect(body.theme).toBeNull()
    expect(body.ifUnmodifiedSince).toBe("2026-05-20T12:34:56Z")
  })

  it("does not fetch branding when the caller pins a theme", async () => {
    const ff = fakeFetch([subscriberFieldsOk, templateSaveOk])
    const out = await runVisualTemplateSave({
      slug: "acme",
      templateId: "t-1",
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

  it("fails closed with 502 when subscriber-fields fetch fails", async () => {
    const ff = fakeFetch([
      on("GET", "/subscriber-fields", () => json({ error: "boom" }, 500)),
    ])
    const out = await runVisualTemplateSave({
      slug: "acme",
      templateId: "t-1",
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
    const out = await runVisualTemplateSave({
      slug: "acme",
      templateId: "t-1",
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
    }
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
    const out = await runVisualTemplateSave({
      slug: "acme",
      templateId: "t-1",
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

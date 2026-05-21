import { describe, expect, it, vi } from "vitest"
import { runRenderPreview } from "./render-preview"
import type { VisualDoc } from "../render/types"

type FakeRoute = (
  url: string,
  init: RequestInit,
) => Promise<Response> | Response | undefined

function fakeFetch(routes: Array<FakeRoute>): {
  fetch: typeof fetch
  calls: Array<{ url: string; method: string; headers: Headers; body: string | null }>
} {
  const calls: Array<{
    url: string
    method: string
    headers: Headers
    body: string | null
  }> = []
  const impl = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const i = init ?? {}
    calls.push({
      url,
      method: i.method ?? "GET",
      headers: new Headers(i.headers),
      body: (i.body as string | null) ?? null,
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
const substituteOk = on("POST", "/substitute-sample", () =>
  json({ html: "<p>Hi Sam</p>", text: "Hi Sam" }),
)

const minimalDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }],
}

describe("runRenderPreview", () => {
  it("happy path: validates, fetches branding, renders, returns 200", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk])
    const out = await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: { bodyDoc: minimalDoc, theme: null },
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(200)
    expect(out.kind).toBe("ok")
    if (out.kind === "ok") {
      expect(out.body.bodyHtml).toContain("hi")
      expect(out.body.bodyText).toContain("hi")
    }
    // Never calls Go's save endpoint.
    expect(ff.calls.map((c) => c.url)).not.toContainEqual(
      expect.stringContaining("/visual"),
    )
  })

  it("does NOT side-call substitute-sample when no sample is provided", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk])
    await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: { bodyDoc: minimalDoc, theme: null },
      fetchImpl: ff.fetch,
    })
    expect(ff.calls.map((c) => c.url)).not.toContainEqual(
      expect.stringContaining("/substitute-sample"),
    )
  })

  it("DOES side-call substitute-sample when sample is provided", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk, substituteOk])
    const out = await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: {
        bodyDoc: minimalDoc,
        theme: null,
        sample: {
          subscriber: { first_name: "Sam" },
          campaign: {},
        },
      },
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(200)
    if (out.kind === "ok") {
      expect(out.body.bodyHtml).toContain("Hi Sam")
      expect(out.body.bodyText).toContain("Hi Sam")
    }
    const substituteCall = ff.calls.find((c) => c.url.endsWith("/substitute-sample"))
    expect(substituteCall).toBeDefined()
    expect(substituteCall!.method).toBe("POST")
    const sent = JSON.parse(substituteCall!.body!)
    expect(sent.sample.subscriber).toEqual({ first_name: "Sam" })
  })

  it("fails closed with 502 when subscriber-fields fetch fails", async () => {
    const ff = fakeFetch([
      on("GET", "/subscriber-fields", () => json({ error: "boom" }, 500)),
    ])
    const out = await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: { bodyDoc: minimalDoc, theme: null },
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(502)
    expect(out.kind).toBe("bad_gateway")
  })

  it("fails closed with 502 when substitute-sample side-call fails", async () => {
    const ff = fakeFetch([
      subscriberFieldsOk,
      brandingOk,
      on("POST", "/substitute-sample", () => json({ error: "down" }, 500)),
    ])
    const out = await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: {
        bodyDoc: minimalDoc,
        theme: null,
        sample: { subscriber: {}, campaign: {} },
      },
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(502)
    expect(out.kind).toBe("bad_gateway")
  })

  it("rejects an unknown subscriber slug before render", async () => {
    const ff = fakeFetch([subscriberFieldsOk])
    const out = await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: {
        bodyDoc: {
          version: 1,
          type: "doc",
          content: [
            {
              type: "paragraph",
              content: [
                { type: "mergeTag", attrs: { namespace: "subscriber", key: "unknown" } },
              ],
            },
          ],
        },
        theme: null,
      },
      fetchImpl: ff.fetch,
    })
    expect(out.status).toBe(400)
    if (out.kind === "validation_error") {
      expect(out.body.kind).toBe("unknown_placeholder")
    }
    // No branding fetch happened because validation aborted early.
    expect(ff.calls.map((c) => c.url)).not.toContainEqual(
      expect.stringContaining("/branding"),
    )
  })

  it("never persists — no PUT to the visual save endpoint", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk])
    await runRenderPreview({
      slug: "acme",
      cookie: "c=x",
      requestId: "req-1",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: { bodyDoc: minimalDoc, theme: null },
      fetchImpl: ff.fetch,
    })
    expect(ff.calls.map((c) => c.method)).not.toContain("PUT")
  })

  it("forwards cookie + X-Request-Id on every Go side-call", async () => {
    const ff = fakeFetch([subscriberFieldsOk, brandingOk, substituteOk])
    await runRenderPreview({
      slug: "acme",
      cookie: "nv_workspace=abc",
      requestId: "req-trace",
      goApiBaseUrl: "http://go.test",
      mediaUrlPrefix: "",
      body: {
        bodyDoc: minimalDoc,
        theme: null,
        sample: { subscriber: { first_name: "Sam" }, campaign: {} },
      },
      fetchImpl: ff.fetch,
    })
    for (const c of ff.calls) {
      expect(c.headers.get("Cookie")).toBe("nv_workspace=abc")
      expect(c.headers.get("X-Request-Id")).toBe("req-trace")
    }
  })
})

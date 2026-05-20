import { describe, expect, it, vi } from "vitest"
import {
  GoApiError,
  GoApiUnreachable,
  createGoApiClient,
} from "./go-api"

function fakeFetch(impl: (url: string, init: RequestInit) => Response | Promise<Response>) {
  return vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
    Promise.resolve(impl(String(input), init ?? {})),
  ) as unknown as typeof fetch
}

describe("createGoApiClient", () => {
  it("forwards the session cookie and X-Request-Id on every call", async () => {
    const calls: Array<{ url: string; headers: Headers; method: string }> = []
    const fetchImpl = fakeFetch((url, init) => {
      calls.push({
        url,
        method: init.method ?? "GET",
        headers: new Headers(init.headers),
      })
      return new Response(JSON.stringify({ fields: [] }), { status: 200 })
    })

    const client = createGoApiClient({
      baseUrl: "http://go.test",
      requestId: "req-abc",
      fetchImpl,
    })
    await client.listSubscriberFields("nv_workspace=cookie-value", "acme")

    expect(calls).toHaveLength(1)
    expect(calls[0].method).toBe("GET")
    expect(calls[0].url).toBe("http://go.test/t/acme/api/subscriber-fields")
    expect(calls[0].headers.get("Cookie")).toBe("nv_workspace=cookie-value")
    expect(calls[0].headers.get("X-Request-Id")).toBe("req-abc")
  })

  it("bubbles a 4xx response as GoApiError carrying the body", async () => {
    const fetchImpl = fakeFetch(() =>
      new Response(
        JSON.stringify({
          error: "stale_row",
          kind: "stale_row",
          currentUpdatedAt: "2026-05-21T00:00:00Z",
        }),
        { status: 409 },
      ),
    )
    const client = createGoApiClient({
      baseUrl: "http://go.test",
      requestId: "req-abc",
      fetchImpl,
    })

    try {
      await client.putCampaignVisual("c=x", "acme", "id-1", {
        subject: "x",
        bodyDoc: { version: 1, type: "doc", content: [] },
        bodyHtml: "<p>x</p>",
        bodyText: "x",
        theme: null,
        ifUnmodifiedSince: "2026-05-20T12:34:56Z",
      })
      throw new Error("expected GoApiError")
    } catch (err) {
      expect(err).toBeInstanceOf(GoApiError)
      const ge = err as GoApiError
      expect(ge.status).toBe(409)
      const body = ge.body as { kind: string; currentUpdatedAt: string }
      expect(body.kind).toBe("stale_row")
      expect(body.currentUpdatedAt).toBe("2026-05-21T00:00:00Z")
    }
  })

  it("throws GoApiUnreachable when fetch itself rejects", async () => {
    const fetchImpl = vi.fn(() =>
      Promise.reject(new Error("ECONNREFUSED")),
    ) as unknown as typeof fetch

    const client = createGoApiClient({
      baseUrl: "http://go.test",
      requestId: "req-abc",
      fetchImpl,
    })

    await expect(client.getBranding("c=x", "acme")).rejects.toBeInstanceOf(
      GoApiUnreachable,
    )
  })

  it("substituteSample POSTs to the tenant-scoped path with a JSON body", async () => {
    const calls: Array<{ url: string; method: string; body: string | null }> = []
    const fetchImpl = fakeFetch((url, init) => {
      calls.push({
        url,
        method: init.method ?? "GET",
        body: (init.body as string | null) ?? null,
      })
      return new Response(JSON.stringify({ html: "Hi Sam", text: "Hi Sam" }), {
        status: 200,
      })
    })

    const client = createGoApiClient({
      baseUrl: "http://go.test",
      requestId: "req-abc",
      fetchImpl,
    })
    const out = await client.substituteSample("c=x", "acme", {
      html: "Hi {{ subscriber.first_name }}",
      text: "Hi {{ subscriber.first_name }}",
      sample: {
        subscriber: { first_name: "Sam" },
        campaign: {},
      },
    })

    expect(out).toEqual({ html: "Hi Sam", text: "Hi Sam" })
    expect(calls[0].url).toBe("http://go.test/t/acme/api/substitute-sample")
    expect(calls[0].method).toBe("POST")
    expect(JSON.parse(calls[0].body!)).toMatchObject({
      sample: { subscriber: { first_name: "Sam" } },
    })
  })
})

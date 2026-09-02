import { afterEach, describe, expect, it, vi } from "vitest"
import { api } from "./api"
import { ApiError } from "./errors"

type Captured = { url: string; init: RequestInit }

// The hand-rolled platform/workspace/etc. transport (src/lib/api.ts's
// `request()`) calls `fetch(url, init)` with a plain string. The generated
// tenant client (openapi-fetch, src/lib/tenant-client.ts) instead builds a
// `Request` object itself and calls `fetch(request, requestInitExt)` — so this
// helper normalizes both calling conventions to the same `{ url, init }`
// shape callers already assert against.
async function capture(
  input: string | Request,
  init?: RequestInit,
): Promise<Captured> {
  if (typeof input === "string") return { url: input, init: init ?? {} }

  const url = new URL(input.url)
  return {
    url: url.pathname + url.search,
    init: {
      method: input.method,
      body: input.body ? await input.clone().text() : undefined,
      // Carried through so the session-cookie contract is assertable: the
      // browser only sends the workspace cookie because tenant-client sets
      // `credentials: "include"`, and reading it off the built Request is the
      // only way to see it once openapi-fetch has folded init into a Request.
      credentials: input.credentials,
      headers: Object.fromEntries(input.headers),
    },
  }
}

function mockFetch(status: number, body: unknown): Array<Captured> {
  const calls: Array<Captured> = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | Request, init?: RequestInit) => {
      const captured = await capture(input, init)
      calls.push(captured)

      const res = new Response(body === null ? null : JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      })
      // A real fetch sets `Response.url` to the absolute final request URL; a
      // hand-built Response leaves it "". tenant-client's unwrap() derives
      // ApiError.path from it — and errors.ts routes a lapsed workspace
      // session off that path — so model it faithfully rather than letting the
      // mock silently exercise the empty-url fallback.
      Object.defineProperty(res, "url", {
        value: new URL(captured.url, window.location.href).toString(),
      })
      return res
    }),
  )
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("api client — verb & path correctness", () => {
  it("posts platform signup", async () => {
    const calls = mockFetch(200, { user: {}, tenants: [] })
    await api.signup("a@b.com", "secret123", "Ann")
    expect(calls[0].url).toBe("/api/platform/signup")
    expect(calls[0].init.method).toBe("POST")
  })

  it("interpolates the slug into tenant-scoped paths", async () => {
    const calls = mockFetch(200, { tenant: { name: "W" }, members: [] })
    await api.tenant("acme")
    expect(calls[0].url).toBe("/t/acme/api/tenant")
    expect(calls[0].init.method).toBe("GET")
  })

  it("builds a paged lists path", async () => {
    const calls = mockFetch(200, { items: [], total: 0 })
    await api.listLists("acme", { limit: 25, offset: 50 })
    expect(calls[0].url).toBe("/t/acme/api/lists?limit=25&offset=50")
  })

  it("sends the segment in the query body", async () => {
    const calls = mockFetch(200, { items: [], total: 0 })
    await api.querySubscribers("acme", { conj: "and", children: [] })
    expect(calls[0].url).toBe("/t/acme/api/subscribers/query")
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      segment: { conj: "and", children: [] },
    })
  })

  it("deletes a list by id", async () => {
    const calls = mockFetch(204, null)
    await api.deleteList("acme", "list-1")
    expect(calls[0].url).toBe("/t/acme/api/lists/list-1")
    expect(calls[0].init.method).toBe("DELETE")
  })
})

// The parts of a serialized multipart/form-data body, in wire order.
function multipartParts(body: string): Array<{ name: string; value: string }> {
  const boundary = body.slice(0, body.indexOf("\r\n"))
  return body
    .split(boundary)
    .filter((part) => part.includes("Content-Disposition"))
    .map((part) => ({
      name: /name="([^"]+)"/.exec(part)![1],
      value: part.slice(part.indexOf("\r\n\r\n") + 4).replace(/\r\n$/, ""),
    }))
}

describe("api client — multipart import", () => {
  it("sends file and repeated listIds fields", async () => {
    const calls = mockFetch(202, { jobId: "job-1" })
    const file = new File(["email\na@b.com"], "people.csv", {
      type: "text/csv",
    })
    await api.startImport("acme", file, ["list-1", "list-2"])
    expect(calls[0].url).toBe("/t/acme/api/import")

    const parts = multipartParts(calls[0].init.body as string)
    // The uploaded file's bytes and filename do not survive here: jsdom's File
    // is invisible to Node's multipart serializer (two separate
    // implementations of the same web APIs), so it encodes an empty part. What
    // the field *names* are is the contract that changed in this migration —
    // `list_ids` became `listIds` — and that is faithfully on the wire.
    expect(parts.map((p) => p.name)).toEqual(["file", "listIds", "listIds"])
    expect(
      parts.filter((p) => p.name === "listIds").map((p) => p.value),
    ).toEqual(["list-1", "list-2"])
  })
})

describe("api client — error normalization", () => {
  it("raises ApiError with status and slug from the problem document", async () => {
    mockFetch(409, { code: "duplicate_email", detail: "Email in use." })
    await expect(api.createSubscriber("acme", {
      email: "a@b.com",
      name: "",
      attributes: {},
      listIds: [],
    })).rejects.toMatchObject({
      status: 409,
      slug: "duplicate_email",
      message: "Email in use.",
    })
  })

  it("normalizes a non-2xx with no body to a default message", async () => {
    mockFetch(500, null)
    try {
      await api.me()
      expect.unreachable("should have thrown")
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError)
      expect((e as ApiError).status).toBe(500)
    }
  })
})

describe("api client — visual editor (Phase 7)", () => {
  const sampleDoc = {
    version: 1 as const,
    type: "doc" as const,
    content: [],
  }

  it("GETs the subscriber-field registry under the tenant slug", async () => {
    const calls = mockFetch(200, { fields: [] })
    await api.subscriberFields.list("acme")
    expect(calls[0].url).toBe("/t/acme/api/subscriber-fields")
    expect(calls[0].init.method).toBe("GET")
  })

  it("creates a subscriber field", async () => {
    const calls = mockFetch(201, {
      id: "f1",
      slug: "country",
      displayName: "Country",
      type: "text",
      defaultValue: "",
      position: 0,
      builtIn: false,
    })
    await api.subscriberFields.create("acme", {
      slug: "country",
      displayName: "Country",
      type: "text",
      defaultValue: "",
    })
    expect(calls[0].url).toBe("/t/acme/api/subscriber-fields")
    expect(calls[0].init.method).toBe("POST")
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      slug: "country",
      displayName: "Country",
      type: "text",
      defaultValue: "",
    })
  })

  it("PATCHes a single field by id", async () => {
    const calls = mockFetch(200, {})
    await api.subscriberFields.update("acme", "f1", {
      displayName: "Country / region",
    })
    expect(calls[0].url).toBe("/t/acme/api/subscriber-fields/f1")
    expect(calls[0].init.method).toBe("PATCH")
  })

  it("DELETEs a field by id", async () => {
    const calls = mockFetch(204, null)
    await api.subscriberFields.delete("acme", "f1")
    expect(calls[0].url).toBe("/t/acme/api/subscriber-fields/f1")
    expect(calls[0].init.method).toBe("DELETE")
  })

  it("PATCHes the order of fields", async () => {
    const calls = mockFetch(200, { fields: [] })
    await api.subscriberFields.reorder("acme", ["f1", "f2"])
    expect(calls[0].url).toBe("/t/acme/api/subscriber-fields/order")
    expect(calls[0].init.method).toBe("PATCH")
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      order: ["f1", "f2"],
    })
  })

  it("lists merge tags", async () => {
    const calls = mockFetch(200, { subscriber: [], campaign: [] })
    await api.mergeTags.list("acme")
    expect(calls[0].url).toBe("/t/acme/api/merge-tags")
    expect(calls[0].init.method).toBe("GET")
  })

  it("PUTs the campaign visual-save with the ifUnmodifiedSince echo", async () => {
    const calls = mockFetch(200, {
      id: "c1",
      subject: "s",
      bodyHtml: "",
      bodyText: "",
      bodyDoc: sampleDoc,
      theme: null,
      warnings: [],
      updatedAt: "2026-05-20T12:35:01Z",
    })
    await api.campaigns.saveVisual("acme", "c1", {
      subject: "Hi",
      bodyDoc: sampleDoc,
      theme: null,
      ifUnmodifiedSince: "2026-05-20T12:34:56Z",
    })
    expect(calls[0].url).toBe("/t/acme/api/campaigns/c1/visual")
    expect(calls[0].init.method).toBe("PUT")
    const body = JSON.parse(calls[0].init.body as string)
    expect(body.ifUnmodifiedSince).toBe("2026-05-20T12:34:56Z")
    expect(body.bodyDoc).toEqual(sampleDoc)
    expect(body.theme).toBeNull()
  })

  it("PUTs the template visual-save", async () => {
    const calls = mockFetch(200, {})
    await api.templates.saveVisual("acme", "t1", {
      name: "Welcome",
      kind: "campaign",
      subject: "Hi",
      bodyDoc: sampleDoc,
      theme: null,
      ifUnmodifiedSince: "2026-05-20T12:34:56Z",
    })
    expect(calls[0].url).toBe("/t/acme/api/templates/t1/visual")
    expect(calls[0].init.method).toBe("PUT")
  })

  it("POSTs render-preview tenant-scoped (no row id)", async () => {
    const calls = mockFetch(200, { bodyHtml: "", bodyText: "", warnings: [] })
    await api.renderPreview("acme", {
      bodyDoc: sampleDoc,
      theme: null,
      sample: null,
    })
    expect(calls[0].url).toBe("/t/acme/api/render-preview")
    expect(calls[0].init.method).toBe("POST")
  })

  it("surfaces the 409 stale_row payload via ApiError", async () => {
    mockFetch(409, {
      code: "stale_row",
      detail: "Row changed in another tab",
      currentUpdatedAt: "2026-05-20T12:35:01Z",
    })
    await expect(
      api.campaigns.saveVisual("acme", "c1", {
        subject: "Hi",
        bodyDoc: sampleDoc,
        theme: null,
        ifUnmodifiedSince: "2026-05-20T12:34:56Z",
      }),
    ).rejects.toMatchObject({
      status: 409,
      slug: "stale_row",
    })
  })
})

describe("audience client", () => {
  it("requests the tenant base path and returns the items envelope", async () => {
    const calls = mockFetch(200, { items: [{ id: "1", name: "L" }], total: 1 })
    const res = await api.listLists("acme", { limit: 10, offset: 0 })

    expect(calls[0].url).toContain("/t/acme/api/lists")
    expect(res.data.items).toHaveLength(1)
    expect(res.data.total).toBe(1)
  })

  it("throws an ApiError carrying the problem code", async () => {
    mockFetch(404, {
      type: "https://nvelope.dev/probs/list_not_found",
      title: "List not found",
      status: 404,
      detail: "no such list",
      code: "list_not_found",
    })
    await expect(api.getList("acme", "missing")).rejects.toMatchObject({
      status: 404,
      slug: "list_not_found",
    })
  })

  it("sends the workspace session cookie and a JSON content type", async () => {
    const calls = mockFetch(201, { id: "list-2" })
    await api.createList("acme", {
      name: "Promotions",
      description: "",
      visibility: "private",
      optIn: "single",
    })
    expect(calls[0].init.credentials).toBe("include")
    expect(calls[0].init.headers).toMatchObject({
      "content-type": "application/json",
    })
  })

  it("carries the tenant path on the error so a lapsed session can be routed", async () => {
    mockFetch(401, { code: "session_expired", detail: "Session expired" })
    await expect(api.getList("acme", "list-1")).rejects.toMatchObject({
      status: 401,
      path: "/t/acme/api/lists/list-1",
    })
  })
})

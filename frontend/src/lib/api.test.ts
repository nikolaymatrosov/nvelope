import { afterEach, describe, expect, it, vi } from "vitest"
import { api } from "./api"
import { ApiError } from "./errors"

type Captured = { url: string; init: RequestInit }

function mockFetch(status: number, body: unknown): Array<Captured> {
  const calls: Array<Captured> = []
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init: RequestInit) => {
      calls.push({ url, init })
      return Promise.resolve(
        new Response(body === null ? null : JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
      )
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
    const calls = mockFetch(200, { lists: [], total: 0 })
    await api.listLists("acme", { limit: 25, offset: 50 })
    expect(calls[0].url).toBe("/t/acme/api/lists?limit=25&offset=50")
  })

  it("sends the segment in the query body", async () => {
    const calls = mockFetch(200, { subscribers: [], total: 0 })
    await api.querySubscribers("acme", { Conj: "and", Children: [] })
    expect(calls[0].url).toBe("/t/acme/api/subscribers/query")
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      segment: { Conj: "and", Children: [] },
    })
  })

  it("deletes a list by id", async () => {
    const calls = mockFetch(204, null)
    await api.deleteList("acme", "list-1")
    expect(calls[0].url).toBe("/t/acme/api/lists/list-1")
    expect(calls[0].init.method).toBe("DELETE")
  })
})

describe("api client — multipart import", () => {
  it("sends file and repeated list_ids fields", async () => {
    const calls = mockFetch(202, { job_id: "job-1" })
    const file = new File(["email\na@b.com"], "people.csv", {
      type: "text/csv",
    })
    await api.startImport("acme", file, ["list-1", "list-2"])
    expect(calls[0].url).toBe("/t/acme/api/import")
    const form = calls[0].init.body as FormData
    expect(form.get("file")).toBeInstanceOf(File)
    expect(form.getAll("list_ids")).toEqual(["list-1", "list-2"])
  })
})

describe("api client — error normalization", () => {
  it("raises ApiError with status and slug from the envelope", async () => {
    mockFetch(409, { error: "duplicate_email", message: "Email in use." })
    await expect(api.createSubscriber("acme", {
      email: "a@b.com",
      name: "",
      attributes: {},
      list_ids: [],
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

describe("api client — PascalCase responses pass through unchanged", () => {
  it("returns audience view fields verbatim", async () => {
    mockFetch(200, {
      lists: [{ ID: "l1", Name: "Newsletter", Tags: [] }],
      total: 1,
    })
    const res = await api.listLists("acme")
    expect(res.data.lists[0].ID).toBe("l1")
    expect(res.data.lists[0].Name).toBe("Newsletter")
  })
})

import { describe, expect, it } from "vitest"
import { ApiError, normalizeError } from "./errors"

const problem = {
  type: "https://nvelope.dev/probs/list_not_found",
  title: "List not found",
  status: 404,
  detail: "no such list",
  instance: "/t/acme/api/lists/abc",
  code: "list_not_found",
  requestId: "req-123",
}

describe("normalizeError", () => {
  it("reads code and detail from a problem document", () => {
    const err = normalizeError(404, problem, "/t/acme/api/lists/abc")
    expect(err).toBeInstanceOf(ApiError)
    expect(err.slug).toBe("list_not_found")
    expect(err.message).toBe("no such list")
    expect(err.status).toBe(404)
  })

  it("falls back to title when detail is absent", () => {
    const { detail: _detail, ...noDetail } = problem
    expect(normalizeError(404, noDetail, "/x").message).toBe("List not found")
  })

  it("exposes extension members through data", () => {
    const err = normalizeError(409, { ...problem, currentUpdatedAt: "t" }, "/x")
    expect(err.data?.currentUpdatedAt).toBe("t")
    expect(err.data?.requestId).toBe("req-123")
  })

  it("falls back to a status message for a non-object body", () => {
    expect(normalizeError(500, null, "/x").message).toBeTruthy()
    expect(normalizeError(500, null, "/x").slug).toBe("")
  })
})

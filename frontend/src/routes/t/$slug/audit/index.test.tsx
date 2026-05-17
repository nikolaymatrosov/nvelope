import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"
import { AuditView } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
}))

vi.mock("@/lib/api", () => ({ api: { auditTrail: vi.fn() } }))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("AuditView", () => {
  it("renders audit records", async () => {
    vi.mocked(api.auditTrail).mockResolvedValue(
      ok({
        records: [
          {
            ID: "a1",
            ActorID: "user-1",
            ActorKind: "session",
            Action: "list.created",
            Target: "list-9",
            Metadata: {},
            CreatedAt: "2026-01-02T10:00:00Z",
          },
        ],
        total: 1,
      }),
    )
    renderWithClient(<AuditView />)
    expect(await screen.findByText("list.created")).toBeDefined()
    expect(screen.getByText("list-9")).toBeDefined()
  })

  it("shows an empty state when there is no activity", async () => {
    vi.mocked(api.auditTrail).mockResolvedValue(ok({ records: [], total: 0 }))
    renderWithClient(<AuditView />)
    expect(await screen.findByText(/no activity yet/i)).toBeDefined()
  })
})

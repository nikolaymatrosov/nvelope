import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { SuppressionsView } from "./index"
import type { SuppressionEntry } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  Link: ({ children }: { children: unknown }) => (
    <a href="#">{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    suppressions: { list: vi.fn(), add: vi.fn(), remove: vi.fn() },
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T, status = 200) => ({ status, ok: true, data })

const entry = (over: Partial<SuppressionEntry> = {}): SuppressionEntry => ({
  email: "bad@example.com",
  reason: "hard_bounce",
  suppressedAt: "2026-05-01T00:00:00Z",
  note: "",
  ...over,
})

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com", locale: null }, tenants: [] }),
  )
  vi.mocked(api.tenant).mockResolvedValue(
    ok({
      tenant: { name: "Acme" },
      members: [
        { user_id: "u1", email: "ann@ex.com", name: "Ann", role: "Owner" },
      ],
    }),
  )
  vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("SuppressionsView", () => {
  it("lists entries with reason and date", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list).mockResolvedValue(
      ok({ items: [entry()], nextCursor: null }),
    )
    renderWithClient(<SuppressionsView />)
    expect(await screen.findByText("bad@example.com")).toBeDefined()
    expect(screen.getByText("Hard bounce")).toBeDefined()
    expect(screen.getByText(/suppressed/i)).toBeDefined()
  })

  it("shows the empty state when there are no entries", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list).mockResolvedValue(
      ok({ items: [], nextCursor: null }),
    )
    renderWithClient(<SuppressionsView />)
    expect(await screen.findByText(/no suppressed addresses/i)).toBeDefined()
  })

  it("loads more entries via the cursor", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list)
      .mockResolvedValueOnce(
        ok({ items: [entry({ email: "one@example.com" })], nextCursor: "c1" }),
      )
      .mockResolvedValueOnce(
        ok({ items: [entry({ email: "two@example.com" })], nextCursor: null }),
      )
    renderWithClient(<SuppressionsView />)
    fireEvent.click(await screen.findByRole("button", { name: /load more/i }))
    expect(await screen.findByText("two@example.com")).toBeDefined()
    expect(api.suppressions.list).toHaveBeenLastCalledWith(
      "acme",
      expect.objectContaining({ cursor: "c1" }),
    )
  })

  it("rejects an invalid email on manual add", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list).mockResolvedValue(
      ok({ items: [], nextCursor: null }),
    )
    renderWithClient(<SuppressionsView />)
    fireEvent.click(
      (await screen.findAllByRole("button", { name: /add address/i }))[0],
    )
    const dialog = await screen.findByRole("dialog")
    const input = within(dialog).getByLabelText(/email address/i)
    fireEvent.change(input, { target: { value: "not-an-email" } })
    fireEvent.blur(input)
    expect(await within(dialog).findByRole("alert")).toBeDefined()
    expect(api.suppressions.add).not.toHaveBeenCalled()
  })

  it("adds a valid address", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list).mockResolvedValue(
      ok({ items: [], nextCursor: null }),
    )
    vi.mocked(api.suppressions.add).mockResolvedValue(
      ok(
        {
          email: "new@example.com",
          reason: "manual" as const,
          note: "",
          suppressedAt: "2026-05-18T00:00:00Z",
        },
        201,
      ),
    )
    renderWithClient(<SuppressionsView />)
    fireEvent.click(
      (await screen.findAllByRole("button", { name: /add address/i }))[0],
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/email address/i), {
      target: { value: "new@example.com" },
    })
    fireEvent.click(
      within(dialog).getByRole("button", { name: /add address/i }),
    )
    await waitFor(() =>
      expect(api.suppressions.add).toHaveBeenCalledWith(
        "acme",
        "new@example.com",
      ),
    )
  })

  it("removes an entry after confirmation", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list).mockResolvedValue(
      ok({ items: [entry()], nextCursor: null }),
    )
    vi.mocked(api.suppressions.remove).mockResolvedValue(ok(null, 204))
    renderWithClient(<SuppressionsView />)
    fireEvent.click(
      await screen.findByRole("button", { name: /remove bad@example.com/i }),
    )
    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(within(dialog).getByRole("button", { name: /remove/i }))
    await waitFor(() =>
      expect(api.suppressions.remove).toHaveBeenCalledWith(
        "acme",
        "bad@example.com",
      ),
    )
  })

  it("reconciles silently when removal hits a 404 race", async () => {
    setupOwner()
    vi.mocked(api.suppressions.list).mockResolvedValue(
      ok({ items: [entry()], nextCursor: null }),
    )
    vi.mocked(api.suppressions.remove).mockRejectedValue(
      new ApiError(404, "suppression_not_found", "Not found", "/x"),
    )
    renderWithClient(<SuppressionsView />)
    fireEvent.click(
      await screen.findByRole("button", { name: /remove bad@example.com/i }),
    )
    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(within(dialog).getByRole("button", { name: /remove/i }))
    await waitFor(() => expect(api.suppressions.remove).toHaveBeenCalled())
  })
})

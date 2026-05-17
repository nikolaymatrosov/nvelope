import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { SubscriberDetail } from "./$id"
import { SubscribersView } from "./index"
import { ApiError } from "@/lib/errors"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "sub-1" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    searchSubscribers: vi.fn(),
    querySubscribers: vi.fn(),
    createSubscriber: vi.fn(),
    getSubscriber: vi.fn(),
    deleteSubscriber: vi.fn(),
    updateSubscriber: vi.fn(),
    listLists: vi.fn(),
    addToList: vi.fn(),
    removeFromList: vi.fn(),
    changeSubscription: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const sampleSubscriber = {
  ID: "sub-1",
  Email: "ann@example.com",
  Name: "Ann",
  State: "enabled",
  Attributes: {},
  Memberships: [],
  CreatedAt: "2026-01-01T00:00:00Z",
  UpdatedAt: "2026-01-01T00:00:00Z",
}

// An Owner has every permission, so management actions are visible.
function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com" }, tenants: [] }),
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

describe("SubscribersView — create", () => {
  it("creates a subscriber through the dialog", async () => {
    setupOwner()
    vi.mocked(api.searchSubscribers).mockResolvedValue(
      ok({ subscribers: [], total: 0 }),
    )
    vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
    vi.mocked(api.createSubscriber).mockResolvedValue(ok({ id: "sub-2" }))
    renderWithClient(<SubscribersView />)

    fireEvent.click(
      await screen.findByRole("button", { name: /new subscriber/i }),
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/email/i), {
      target: { value: "new@example.com" },
    })
    fireEvent.click(
      within(dialog).getByRole("button", { name: /create subscriber/i }),
    )

    await waitFor(() =>
      expect(api.createSubscriber).toHaveBeenCalledWith("acme", {
        email: "new@example.com",
        name: "",
        attributes: {},
        list_ids: [],
      }),
    )
  })

  it("surfaces a duplicate-email conflict", async () => {
    setupOwner()
    vi.mocked(api.searchSubscribers).mockResolvedValue(
      ok({ subscribers: [], total: 0 }),
    )
    vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
    vi.mocked(api.createSubscriber).mockRejectedValue(
      new ApiError(409, "duplicate_email", "exists", "/t/acme/api/subscribers"),
    )
    renderWithClient(<SubscribersView />)

    fireEvent.click(
      await screen.findByRole("button", { name: /new subscriber/i }),
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/email/i), {
      target: { value: "dupe@example.com" },
    })
    fireEvent.click(
      within(dialog).getByRole("button", { name: /create subscriber/i }),
    )

    expect(
      await screen.findByText(/a subscriber with this email already exists/i),
    ).toBeDefined()
  })
})

describe("SubscriberDetail — delete", () => {
  it("requires confirmation before deleting", async () => {
    vi.mocked(api.getSubscriber).mockResolvedValue(
      ok({ subscriber: sampleSubscriber }),
    )
    vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
    vi.mocked(api.deleteSubscriber).mockResolvedValue(ok(null))
    renderWithClient(<SubscriberDetail />)

    fireEvent.click(await screen.findByRole("button", { name: /^delete$/i }))
    expect(api.deleteSubscriber).not.toHaveBeenCalled()

    const dialog = await screen.findByRole("alertdialog")
    expect(within(dialog).getByText(/delete this subscriber/i)).toBeDefined()
    fireEvent.click(
      within(dialog).getByRole("button", { name: /delete subscriber/i }),
    )

    await waitFor(() =>
      expect(api.deleteSubscriber).toHaveBeenCalledWith("acme", "sub-1"),
    )
  })
})

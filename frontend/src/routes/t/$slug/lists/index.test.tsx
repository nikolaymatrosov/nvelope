import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { ListDetail } from "./$id"
import { ListsView } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "list-1" }),
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
    listLists: vi.fn(),
    createList: vi.fn(),
    getList: vi.fn(),
    updateList: vi.fn(),
    deleteList: vi.fn(),
    querySubscribers: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const sampleList = {
  ID: "list-1",
  Name: "Newsletter",
  Description: "Weekly digest",
  Visibility: "private",
  OptIn: "single",
  Tags: [],
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

describe("ListsView", () => {
  it("renders existing lists", async () => {
    vi.mocked(api.listLists).mockResolvedValue(
      ok({ lists: [sampleList], total: 1 }),
    )
    renderWithClient(<ListsView />)
    expect(await screen.findByText("Newsletter")).toBeDefined()
  })

  it("creates a list through the dialog", async () => {
    setupOwner()
    vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
    vi.mocked(api.createList).mockResolvedValue(ok({ id: "list-2" }))
    renderWithClient(<ListsView />)

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /new list/i }))[0],
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/name/i), {
      target: { value: "Promotions" },
    })
    fireEvent.click(within(dialog).getByRole("button", { name: /create list/i }))

    await waitFor(() =>
      expect(api.createList).toHaveBeenCalledWith("acme", {
        name: "Promotions",
        description: "",
      }),
    )
  })
})

describe("ListDetail", () => {
  it("requires confirmation before deleting a list", async () => {
    vi.mocked(api.getList).mockResolvedValue(ok({ list: sampleList }))
    vi.mocked(api.querySubscribers).mockResolvedValue(
      ok({ subscribers: [], total: 0 }),
    )
    vi.mocked(api.deleteList).mockResolvedValue(ok(null))
    renderWithClient(<ListDetail />)

    fireEvent.click(await screen.findByRole("button", { name: /delete list/i }))
    expect(api.deleteList).not.toHaveBeenCalled()

    const dialog = await screen.findByRole("alertdialog")
    expect(within(dialog).getByText(/delete this list/i)).toBeDefined()
    fireEvent.click(within(dialog).getByRole("button", { name: /delete list/i }))

    await waitFor(() =>
      expect(api.deleteList).toHaveBeenCalledWith("acme", "list-1"),
    )
  })
})

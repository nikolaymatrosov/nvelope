import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { InvitationsPanel, MembersPanel } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => opts,
  useNavigate: () => vi.fn(),
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    tenant: vi.fn(),
    listRoles: vi.fn(),
    listInvitations: vi.fn(),
    invite: vi.fn(),
    revokeInvitation: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("MembersPanel", () => {
  it("lists workspace members", async () => {
    vi.mocked(api.tenant).mockResolvedValue(
      ok({
        tenant: { name: "Acme" },
        members: [
          { user_id: "u1", email: "ann@ex.com", name: "Ann", role: "Owner" },
        ],
      }),
    )
    vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
    renderWithClient(<MembersPanel slug="acme" canManageRoles />)
    expect(await screen.findByText("Ann")).toBeDefined()
    expect(screen.getByText("ann@ex.com")).toBeDefined()
  })
})

describe("InvitationsPanel", () => {
  it("sends an invitation", async () => {
    vi.mocked(api.listInvitations).mockResolvedValue(ok({ invitations: [] }))
    vi.mocked(api.invite).mockResolvedValue(
      ok({ accept_url: "https://app/invite/tok" }),
    )
    renderWithClient(<InvitationsPanel slug="acme" />)

    fireEvent.change(await screen.findByLabelText(/email address/i), {
      target: { value: "new@ex.com" },
    })
    fireEvent.click(screen.getByRole("button", { name: /send invite/i }))

    await waitFor(() =>
      expect(api.invite).toHaveBeenCalledWith("acme", "new@ex.com"),
    )
  })

  it("revokes a pending invitation", async () => {
    vi.mocked(api.listInvitations).mockResolvedValue(
      ok({
        invitations: [
          {
            id: "inv-1",
            email: "pending@ex.com",
            status: "pending",
            created_at: "2026-01-01T00:00:00Z",
            expires_at: "2026-02-01T00:00:00Z",
          },
        ],
      }),
    )
    vi.mocked(api.revokeInvitation).mockResolvedValue(ok(null))
    renderWithClient(<InvitationsPanel slug="acme" />)

    fireEvent.click(await screen.findByRole("button", { name: /^revoke$/i }))
    expect(api.revokeInvitation).not.toHaveBeenCalled()

    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(
      within(dialog).getByRole("button", { name: /revoke invitation/i }),
    )
    await waitFor(() =>
      expect(api.revokeInvitation).toHaveBeenCalledWith("acme", "inv-1"),
    )
  })
})

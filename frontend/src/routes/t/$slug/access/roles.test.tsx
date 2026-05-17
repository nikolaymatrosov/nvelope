import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { RolesPanel } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => opts,
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: unknown }) => <>{children as never}</>,
}))

vi.mock("@/lib/api", () => ({
  api: { listRoles: vi.fn(), createRole: vi.fn(), deleteRole: vi.fn() },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("RolesPanel", () => {
  it("lets an admin create a role", async () => {
    vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
    vi.mocked(api.createRole).mockResolvedValue(ok({ id: "role-1" }))
    renderWithClient(<RolesPanel slug="acme" canManageRoles />)

    fireEvent.click(await screen.findByRole("button", { name: /new role/i }))
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/role name/i), {
      target: { value: "Editor" },
    })
    fireEvent.click(within(dialog).getByRole("button", { name: /create role/i }))

    await waitFor(() =>
      expect(api.createRole).toHaveBeenCalledWith("acme", "Editor", []),
    )
  })

  it("hides role management from a non-admin", async () => {
    vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
    renderWithClient(<RolesPanel slug="acme" canManageRoles={false} />)

    expect(
      await screen.findByText(/role management unavailable/i),
    ).toBeDefined()
    expect(screen.queryByRole("button", { name: /new role/i })).toBeNull()
  })
})

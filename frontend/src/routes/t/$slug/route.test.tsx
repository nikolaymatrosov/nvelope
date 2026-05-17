import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"
import { WorkspaceLayout } from "./route"
import { ApiError } from "@/lib/errors"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: "/t/acme/lists" }),
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
  Outlet: () => <div data-testid="outlet" />,
}))

vi.mock("@/lib/api", () => ({
  api: {
    openSession: vi.fn(),
    tenant: vi.fn(),
    me: vi.fn(),
    listRoles: vi.fn(),
    logout: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("WorkspaceLayout", () => {
  it("shows the not-found / no-access screen on a 404", async () => {
    vi.mocked(api.openSession).mockRejectedValue(
      new ApiError(404, "not_found", "no", "/t/acme/api/session"),
    )
    renderWithClient(<WorkspaceLayout />)
    expect(
      await screen.findByText(/workspace not available/i),
    ).toBeDefined()
  })

  it("renders the shell with the workspace name and active section", async () => {
    vi.mocked(api.openSession).mockResolvedValue(ok({ state: "active" }))
    vi.mocked(api.tenant).mockResolvedValue(
      ok({
        tenant: { name: "Acme Inc" },
        members: [
          {
            user_id: "u1",
            email: "ann@example.com",
            name: "Ann",
            role: "Owner",
          },
        ],
      }),
    )
    vi.mocked(api.me).mockResolvedValue(
      ok({
        user: { id: "u1", name: "Ann", email: "ann@example.com" },
        tenants: [],
      }),
    )
    vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))

    renderWithClient(<WorkspaceLayout />)

    expect(await screen.findByText("Acme Inc")).toBeDefined()
    const listsNav = await screen.findByRole("link", { name: /lists/i })
    expect(listsNav.getAttribute("data-active")).toBe("true")
    expect(screen.getByTestId("outlet")).toBeDefined()
  })
})

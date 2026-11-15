import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { NewTenant } from "./new"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

const navigate = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: unknown) => opts,
  useNavigate: () => navigate,
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({ api: { createTenant: vi.fn() } }))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("NewTenant", () => {
  it("navigates to the slug the server returned, not the typed one", async () => {
    vi.mocked(api.createTenant).mockResolvedValue({
      status: 201,
      ok: true,
      data: { tenant: { id: "t1", slug: "pinned", name: "Acme", status: "active" } },
    })
    renderWithClient(<NewTenant />)
    fireEvent.change(screen.getByLabelText(/workspace name/i), {
      target: { value: "Acme" },
    })
    fireEvent.change(screen.getByLabelText(/workspace address/i), {
      target: { value: "typed-slug" },
    })
    fireEvent.click(screen.getByRole("button", { name: /create/i }))

    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({
        to: "/t/$slug",
        params: { slug: "pinned" },
      }),
    )
  })
})

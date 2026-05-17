import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { AcceptInvite } from "./$token"
import { ApiError } from "@/lib/errors"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ token: "tok-1" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: { getInvitation: vi.fn(), acceptInvitation: vi.fn() },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("AcceptInvite", () => {
  it("accepts a valid invitation", async () => {
    vi.mocked(api.getInvitation).mockResolvedValue(
      ok({
        email: "invitee@ex.com",
        status: "pending",
        expires_at: "2026-02-01T00:00:00Z",
        tenant: { name: "Acme", slug: "acme" },
      }),
    )
    vi.mocked(api.acceptInvitation).mockResolvedValue(
      ok({ tenant: { slug: "acme" } }),
    )
    renderWithClient(<AcceptInvite />)

    expect(await screen.findByText(/join acme/i)).toBeDefined()
    fireEvent.change(screen.getByLabelText(/your name/i), {
      target: { value: "Pat" },
    })
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: "supersecret" },
    })
    fireEvent.click(screen.getByRole("button", { name: /accept invitation/i }))

    await waitFor(() =>
      expect(api.acceptInvitation).toHaveBeenCalledWith(
        "tok-1",
        "supersecret",
        "Pat",
      ),
    )
  })

  it("shows an invalid screen when the lookup fails", async () => {
    vi.mocked(api.getInvitation).mockRejectedValue(
      new ApiError(404, "not_found", "no", "/api/platform/invitations/tok-1"),
    )
    renderWithClient(<AcceptInvite />)
    expect(await screen.findByText(/invitation not valid/i)).toBeDefined()
  })

  it("shows an invalid screen for an expired invitation", async () => {
    vi.mocked(api.getInvitation).mockResolvedValue(
      ok({
        email: "invitee@ex.com",
        status: "expired",
        expires_at: "2020-01-01T00:00:00Z",
        tenant: { name: "Acme", slug: "acme" },
      }),
    )
    renderWithClient(<AcceptInvite />)
    expect(await screen.findByText(/invitation not valid/i)).toBeDefined()
  })
})

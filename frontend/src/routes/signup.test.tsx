import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { Signup } from "./signup"
import { ApiError } from "@/lib/errors"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: unknown) => opts,
  useNavigate: () => navigate,
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({ api: { signup: vi.fn() } }))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

function fill() {
  fireEvent.change(screen.getByLabelText(/name/i), {
    target: { value: "Ann Lee" },
  })
  fireEvent.change(screen.getByLabelText(/email/i), {
    target: { value: "ann@example.com" },
  })
  fireEvent.change(screen.getByLabelText(/password/i), {
    target: { value: "supersecret" },
  })
}

describe("Signup", () => {
  it("registers an account and routes to workspace creation", async () => {
    vi.mocked(api.signup).mockResolvedValue({
      status: 200,
      ok: true,
      data: { user: { id: "u1", name: "Ann Lee", email: "ann@example.com" }, tenants: [] },
    })
    renderWithClient(<Signup />)
    fill()
    fireEvent.click(screen.getByRole("button", { name: /sign up/i }))

    await waitFor(() =>
      expect(api.signup).toHaveBeenCalledWith(
        "ann@example.com",
        "supersecret",
        "Ann Lee",
      ),
    )
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({ to: "/tenants/new" }),
    )
  })

  it("surfaces a duplicate-email error without creating a second account", async () => {
    vi.mocked(api.signup).mockRejectedValue(
      new ApiError(409, "duplicate_email", "exists", "/api/platform/signup"),
    )
    renderWithClient(<Signup />)
    fill()
    fireEvent.click(screen.getByRole("button", { name: /sign up/i }))

    expect(
      await screen.findByText(/an account with this email already exists/i),
    ).toBeDefined()
    expect(navigate).not.toHaveBeenCalled()
  })
})

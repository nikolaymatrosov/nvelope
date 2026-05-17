import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { Login } from "./login"
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

vi.mock("@/lib/api", () => ({ api: { login: vi.fn() } }))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

function fill() {
  fireEvent.change(screen.getByLabelText(/email/i), {
    target: { value: "ann@example.com" },
  })
  fireEvent.change(screen.getByLabelText(/password/i), {
    target: { value: "supersecret" },
  })
}

describe("Login", () => {
  it("signs in and routes to the workspace picker", async () => {
    vi.mocked(api.login).mockResolvedValue({
      status: 200,
      ok: true,
      data: null,
    })
    renderWithClient(<Login />)
    fill()
    fireEvent.click(screen.getByRole("button", { name: /log in/i }))

    await waitFor(() =>
      expect(api.login).toHaveBeenCalledWith("ann@example.com", "supersecret"),
    )
    await waitFor(() => expect(navigate).toHaveBeenCalledWith({ to: "/" }))
  })

  it("shows a non-specific error on invalid credentials", async () => {
    vi.mocked(api.login).mockRejectedValue(
      new ApiError(401, "unauthenticated", "bad", "/api/platform/login"),
    )
    renderWithClient(<Login />)
    fill()
    fireEvent.click(screen.getByRole("button", { name: /log in/i }))

    expect(
      await screen.findByText(/that email and password do not match/i),
    ).toBeDefined()
    expect(navigate).not.toHaveBeenCalled()
  })
})

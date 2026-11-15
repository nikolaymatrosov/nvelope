import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { Signup } from "./signup"
import { ApiError } from "@/lib/errors"
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

vi.mock("@/lib/api", () => ({ api: { signup: vi.fn(), config: vi.fn() } }))

const saasConfig = {
  status: 200,
  ok: true,
  data: { features: { billing: true, multi_tenant: true }, setup_required: false },
}

beforeEach(() => {
  vi.mocked(api.config).mockResolvedValue(saasConfig)
})

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
  it("registers an account and shows the check-your-inbox screen", async () => {
    vi.mocked(api.signup).mockResolvedValue({
      status: 201,
      ok: true,
      data: { verification: { required: true, email: "ann@example.com" } },
    })
    renderWithClient(<Signup />)
    expect(screen.queryByLabelText(/setup token/i)).toBeNull()
    fill()
    fireEvent.click(screen.getByRole("button", { name: /sign up/i }))

    await waitFor(() =>
      expect(api.signup).toHaveBeenCalledWith(
        "ann@example.com",
        "supersecret",
        "Ann Lee",
      ),
    )
    // No session is issued — the user is told to verify their email instead of
    // being routed into the app.
    expect(await screen.findByText(/check your inbox/i)).toBeDefined()
    expect(await screen.findByText(/ann@example.com/i)).toBeDefined()
  })

  it("in first-run setup, sends the setup token and enters the new workspace", async () => {
    vi.mocked(api.config).mockResolvedValue({
      status: 200,
      ok: true,
      data: { features: { billing: false, multi_tenant: false }, setup_required: true },
    })
    vi.mocked(api.signup).mockResolvedValue({
      status: 201,
      ok: true,
      data: {
        user: { id: "u1", name: "Ann Lee", email: "ann@example.com", locale: null },
        tenant: { id: "t1", slug: "acme", name: "acme", status: "active" },
      },
    })
    renderWithClient(<Signup />)
    const token = await screen.findByLabelText(/setup token/i)
    fill()
    fireEvent.change(token, { target: { value: "the-setup-token" } })
    fireEvent.click(screen.getByRole("button", { name: /^set up$/i }))

    await waitFor(() =>
      expect(api.signup).toHaveBeenCalledWith(
        "ann@example.com",
        "supersecret",
        "Ann Lee",
        "the-setup-token",
      ),
    )
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({
        to: "/t/$slug",
        params: { slug: "acme" },
      }),
    )
  })

  it("surfaces a duplicate-email error without creating a second account", async () => {
    vi.mocked(api.signup).mockRejectedValue(
      new ApiError(409, "email_taken", "exists", "/api/platform/signup"),
    )
    renderWithClient(<Signup />)
    fill()
    fireEvent.click(screen.getByRole("button", { name: /sign up/i }))

    expect(
      await screen.findByText(/an account with this email already exists/i),
    ).toBeDefined()
    expect(screen.queryByText(/check your inbox/i)).toBeNull()
  })
})

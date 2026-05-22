import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen, waitFor } from "@testing-library/react"
import { QueryClientProvider } from "@tanstack/react-query"
import { VerifyEmail } from "./verify-email"
import { ApiError } from "@/lib/errors"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

const { search } = vi.hoisted(() => ({ search: { token: "" } }))

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: Record<string, unknown>) => ({
    ...opts,
    useSearch: () => search,
  }),
  Link: ({ children, to, ...rest }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"} {...rest}>
      {children as never}
    </a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: { verifyEmail: vi.fn(), resendVerification: vi.fn() },
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  search.token = ""
})

describe("VerifyEmail", () => {
  it("verifies a valid token exactly once and shows success", async () => {
    search.token = "good-token"
    vi.mocked(api.verifyEmail).mockResolvedValue({
      status: 200,
      ok: true,
      data: { verification: { status: "verified" } },
    })
    const { client, rerender } = renderWithClient(<VerifyEmail />)

    expect(
      await screen.findByText(/your email address is confirmed/i),
    ).toBeDefined()
    expect(api.verifyEmail).toHaveBeenCalledWith("good-token")

    // A re-render must not fire a second verification request — the POST is a
    // one-shot, not a query that refetches.
    rerender(
      <QueryClientProvider client={client}>
        <VerifyEmail />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(api.verifyEmail).toHaveBeenCalledTimes(1))
  })

  it("reports an account that was already verified", async () => {
    search.token = "used-token"
    vi.mocked(api.verifyEmail).mockResolvedValue({
      status: 200,
      ok: true,
      data: { verification: { status: "already_verified" } },
    })
    renderWithClient(<VerifyEmail />)

    expect(
      await screen.findByText(/already been verified/i),
    ).toBeDefined()
  })

  it("does not call the API when the link is missing its token", async () => {
    renderWithClient(<VerifyEmail />)

    expect(
      await screen.findByText(/missing its token/i),
    ).toBeDefined()
    expect(api.verifyEmail).not.toHaveBeenCalled()
  })

  it("offers a resend when the token is invalid", async () => {
    search.token = "bad-token"
    vi.mocked(api.verifyEmail).mockRejectedValue(
      new ApiError(
        422,
        "verification_link_invalid",
        "bad",
        "/api/platform/verify-email",
      ),
    )
    renderWithClient(<VerifyEmail />)

    expect(
      await screen.findByText(/invalid or has expired/i),
    ).toBeDefined()
    expect(screen.getByLabelText(/email/i)).toBeDefined()
  })
})

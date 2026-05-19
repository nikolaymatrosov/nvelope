import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen, waitFor } from "@testing-library/react"
import { SuspensionBanner } from "./suspension-banner"
import type {
  SubscriptionResponse,
  SubscriptionState,
} from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: unknown }) => (
    <a href="#">{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: { billing: { getSubscription: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function subscription(state: SubscriptionState) {
  const data: SubscriptionResponse = {
    subscription: {
      id: "sub1",
      plan: { id: "p1", code: "pro", name: "Pro", overageMode: "block" },
      state,
      currentPeriodStart: "2026-05-01T00:00:00Z",
      currentPeriodEnd: "2026-06-01T00:00:00Z",
      cancelAtPeriodEnd: false,
    },
    usage: {
      includedSends: 10000,
      usedSends: 0,
      overageSends: 0,
      remainingSends: 10000,
    },
  }
  return ok(data)
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("SuspensionBanner", () => {
  it("shows the banner when the subscription is suspended", async () => {
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("suspended"),
    )
    renderWithClient(<SuspensionBanner slug="acme" />)
    expect(await screen.findByTestId("suspension-banner")).toBeTruthy()
  })

  it("renders nothing for an active subscription", async () => {
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("active"),
    )
    const { container } = renderWithClient(<SuspensionBanner slug="acme" />)
    await waitFor(() => expect(api.billing.getSubscription).toHaveBeenCalled())
    expect(screen.queryByTestId("suspension-banner")).toBeNull()
    expect(container.textContent).toBe("")
  })

  it("renders nothing when there is no subscription", async () => {
    vi.mocked(api.billing.getSubscription).mockRejectedValue(
      new ApiError(404, "no_subscription", "no subscription", "/p"),
    )
    renderWithClient(<SuspensionBanner slug="acme" />)
    await waitFor(() => expect(api.billing.getSubscription).toHaveBeenCalled())
    expect(screen.queryByTestId("suspension-banner")).toBeNull()
  })
})

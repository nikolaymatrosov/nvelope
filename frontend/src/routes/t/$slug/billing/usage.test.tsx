import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"
import { UsagePage } from "./usage"
import type { SubscriptionResponse } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  Link: ({ children }: { children: unknown }) => (
    <a href="#">{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: { billing: { getSubscription: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function usage(opts: {
  overageMode: "block" | "meter"
  includedSends: number
  usedSends: number
  overageSends: number
  remainingSends: number
}) {
  const data: SubscriptionResponse = {
    subscription: {
      id: "sub1",
      plan: {
        id: "p1",
        code: "pro",
        name: "Pro",
        overageMode: opts.overageMode,
      },
      state: "active",
      currentPeriodStart: "2026-05-01T00:00:00Z",
      currentPeriodEnd: "2026-06-01T00:00:00Z",
      cancelAtPeriodEnd: false,
    },
    usage: {
      includedSends: opts.includedSends,
      usedSends: opts.usedSends,
      overageSends: opts.overageSends,
      remainingSends: opts.remainingSends,
    },
  }
  return ok(data)
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("UsagePage", () => {
  it("shows consumed sends against the allowance", async () => {
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      usage({
        overageMode: "block",
        includedSends: 10000,
        usedSends: 2500,
        overageSends: 0,
        remainingSends: 7500,
      }),
    )
    renderWithClient(<UsagePage />)
    expect(await screen.findByText(/2,500/)).toBeTruthy()
    expect(screen.getByTestId("usage-refresh-note")).toBeTruthy()
  })

  it("warns that sends are blocked when a block-mode allowance is exhausted", async () => {
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      usage({
        overageMode: "block",
        includedSends: 10000,
        usedSends: 10000,
        overageSends: 0,
        remainingSends: 0,
      }),
    )
    renderWithClient(<UsagePage />)
    expect(await screen.findByTestId("usage-blocked")).toBeTruthy()
  })

  it("shows overage billing for a meter-mode plan", async () => {
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      usage({
        overageMode: "meter",
        includedSends: 10000,
        usedSends: 11000,
        overageSends: 1000,
        remainingSends: 0,
      }),
    )
    renderWithClient(<UsagePage />)
    expect(await screen.findByTestId("usage-overage")).toBeTruthy()
  })

  it("shows the no-subscription state on a 404", async () => {
    vi.mocked(api.billing.getSubscription).mockRejectedValue(
      new ApiError(404, "no_subscription", "no subscription", "/p"),
    )
    renderWithClient(<UsagePage />)
    expect(await screen.findByTestId("usage-no-subscription")).toBeTruthy()
  })
})

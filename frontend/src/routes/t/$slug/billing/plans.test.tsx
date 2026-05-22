import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { PlansPage } from "./plans"
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
  api: {
    billing: {
      plans: vi.fn(),
      getSubscription: vi.fn(),
      subscribe: vi.fn(),
    },
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com", locale: null }, tenants: [] }),
  )
  vi.mocked(api.tenant).mockResolvedValue(
    ok({
      tenant: { name: "Acme" },
      members: [
        { user_id: "u1", email: "ann@ex.com", name: "Ann", role: "Owner" },
      ],
    }),
  )
  vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
}

const PRO_PLAN = {
  id: "p1",
  code: "pro",
  name: "Pro",
  priceMinor: 500000,
  currency: "RUB",
  billingPeriod: "monthly",
  includedSends: 10000,
  overageMode: "block" as const,
  overagePriceMinor: 0,
}

function noSubscription() {
  vi.mocked(api.billing.getSubscription).mockRejectedValue(
    new ApiError(404, "no_subscription", "no subscription", "/p"),
  )
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("PlansPage", () => {
  it("lists available plans with price and allowance", async () => {
    setupOwner()
    noSubscription()
    vi.mocked(api.billing.plans).mockResolvedValue(ok({ plans: [PRO_PLAN] }))
    renderWithClient(<PlansPage />)
    expect(await screen.findByText("Pro")).toBeTruthy()
    expect(screen.getByText(/10,000 sends included/)).toBeTruthy()
  })

  it("shows an empty state when no plans exist", async () => {
    setupOwner()
    noSubscription()
    vi.mocked(api.billing.plans).mockResolvedValue(ok({ plans: [] }))
    renderWithClient(<PlansPage />)
    expect(await screen.findByTestId("plans-empty")).toBeTruthy()
  })

  it("subscribes after confirming the charge summary", async () => {
    setupOwner()
    noSubscription()
    vi.mocked(api.billing.plans).mockResolvedValue(ok({ plans: [PRO_PLAN] }))
    vi.mocked(api.billing.subscribe).mockResolvedValue(ok({}) as never)
    renderWithClient(<PlansPage />)
    fireEvent.click(await screen.findByRole("button", { name: /subscribe/i }))
    const confirm = await screen.findByRole("button", {
      name: /confirm & pay/i,
    })
    fireEvent.click(confirm)
    await waitFor(() =>
      expect(api.billing.subscribe).toHaveBeenCalledWith("acme", "p1"),
    )
  })

  it("shows a declined-charge warning on payment_failed", async () => {
    setupOwner()
    noSubscription()
    vi.mocked(api.billing.plans).mockResolvedValue(ok({ plans: [PRO_PLAN] }))
    vi.mocked(api.billing.subscribe).mockRejectedValue(
      new ApiError(402, "payment_failed", "declined", "/p"),
    )
    renderWithClient(<PlansPage />)
    fireEvent.click(await screen.findByRole("button", { name: /subscribe/i }))
    fireEvent.click(
      await screen.findByRole("button", { name: /confirm & pay/i }),
    )
    expect(await screen.findByTestId("plans-declined")).toBeTruthy()
  })

  it("disables subscribing when a subscription already exists", async () => {
    setupOwner()
    const active: SubscriptionResponse = {
      subscription: {
        id: "sub1",
        plan: { id: "p1", code: "pro", name: "Pro", overageMode: "block" },
        state: "active",
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
    vi.mocked(api.billing.getSubscription).mockResolvedValue(ok(active))
    vi.mocked(api.billing.plans).mockResolvedValue(ok({ plans: [PRO_PLAN] }))
    renderWithClient(<PlansPage />)
    expect(
      await screen.findByTestId("plans-already-subscribed"),
    ).toBeTruthy()
    const button = await screen.findByRole("button", { name: /subscribe/i })
    expect(button.hasAttribute("disabled")).toBe(true)
  })
})

import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { BillingOverview } from "./index"
import type {
  InvoiceSummary,
  SubscriptionResponse,
  SubscriptionState,
} from "@/lib/api-types"
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
      getSubscription: vi.fn(),
      listInvoices: vi.fn(),
      settleInvoice: vi.fn(),
    },
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com" }, tenants: [] }),
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
      usedSends: 1200,
      overageSends: 0,
      remainingSends: 8800,
    },
  }
  return ok(data)
}

const OPEN_INVOICE: InvoiceSummary = {
  id: "inv1",
  periodStart: "2026-05-01T00:00:00Z",
  periodEnd: "2026-06-01T00:00:00Z",
  totalMinor: 500000,
  currency: "RUB",
  status: "open",
  issuedAt: "2026-05-01T00:00:00Z",
  paidAt: null,
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("BillingOverview", () => {
  it("shows the plan and active state for an active subscription", async () => {
    setupOwner()
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("active"),
    )
    renderWithClient(<BillingOverview />)
    expect((await screen.findAllByText("Pro")).length).toBeGreaterThan(0)
    expect(screen.getByText("Active")).toBeTruthy()
  })

  it("shows the no-subscription state on a 404", async () => {
    setupOwner()
    vi.mocked(api.billing.getSubscription).mockRejectedValue(
      new ApiError(404, "no_subscription", "no subscription", "/p"),
    )
    renderWithClient(<BillingOverview />)
    expect(
      await screen.findByTestId("billing-no-subscription"),
    ).toBeTruthy()
  })

  it("shows an in-progress state for a pending subscription", async () => {
    setupOwner()
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("pending"),
    )
    renderWithClient(<BillingOverview />)
    expect(await screen.findByTestId("billing-pending")).toBeTruthy()
  })

  it("shows a past-due warning and the settle panel", async () => {
    setupOwner()
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("past_due"),
    )
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({ invoices: [OPEN_INVOICE], total: 1 }),
    )
    renderWithClient(<BillingOverview />)
    expect(await screen.findByTestId("billing-past-due")).toBeTruthy()
    expect(await screen.findByTestId("settle-panel")).toBeTruthy()
  })

  it("settles the outstanding balance for a suspended tenant", async () => {
    setupOwner()
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("suspended"),
    )
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({ invoices: [OPEN_INVOICE], total: 1 }),
    )
    vi.mocked(api.billing.settleInvoice).mockResolvedValue(
      ok({}) as never,
    )
    renderWithClient(<BillingOverview />)
    expect(await screen.findByTestId("billing-suspended")).toBeTruthy()
    const button = await screen.findByRole("button", {
      name: /settle balance now/i,
    })
    fireEvent.click(button)
    await waitFor(() =>
      expect(api.billing.settleInvoice).toHaveBeenCalledWith("acme", "inv1"),
    )
  })

  it("keeps the account suspended when the settle charge is declined", async () => {
    setupOwner()
    vi.mocked(api.billing.getSubscription).mockResolvedValue(
      subscription("suspended"),
    )
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({ invoices: [OPEN_INVOICE], total: 1 }),
    )
    vi.mocked(api.billing.settleInvoice).mockRejectedValue(
      new ApiError(402, "payment_failed", "declined", "/p"),
    )
    renderWithClient(<BillingOverview />)
    const button = await screen.findByRole("button", {
      name: /settle balance now/i,
    })
    fireEvent.click(button)
    await waitFor(() =>
      expect(api.billing.settleInvoice).toHaveBeenCalledWith("acme", "inv1"),
    )
    expect(screen.getByTestId("billing-suspended")).toBeTruthy()
  })
})

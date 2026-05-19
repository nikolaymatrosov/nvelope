import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen } from "@testing-library/react"
import { InvoicesPage } from "./invoices"
import type { InvoiceSummary, InvoiceView } from "@/lib/api-types"
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
  api: { billing: { listInvoices: vi.fn(), getInvoice: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function invoiceRow(id: string, status: "open" | "paid"): InvoiceSummary {
  return {
    id,
    periodStart: "2026-04-01T00:00:00Z",
    periodEnd: "2026-05-01T00:00:00Z",
    totalMinor: 500000,
    currency: "RUB",
    status,
    issuedAt: "2026-04-01T00:00:00Z",
    paidAt: status === "paid" ? "2026-04-02T00:00:00Z" : null,
  }
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("InvoicesPage", () => {
  it("lists invoices and distinguishes paid from unpaid", async () => {
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({
        invoices: [invoiceRow("inv1", "paid"), invoiceRow("inv2", "open")],
        total: 2,
      }),
    )
    renderWithClient(<InvoicesPage />)
    const rows = await screen.findAllByTestId("invoice-row")
    expect(rows).toHaveLength(2)
    expect(rows[0].getAttribute("data-status")).toBe("paid")
    expect(rows[1].getAttribute("data-status")).toBe("open")
  })

  it("shows an empty state when there are no invoices", async () => {
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({ invoices: [], total: 0 }),
    )
    renderWithClient(<InvoicesPage />)
    expect(await screen.findByTestId("invoices-empty")).toBeTruthy()
  })

  it("opens an invoice detail with line items and payment attempts", async () => {
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({ invoices: [invoiceRow("inv1", "open")], total: 1 }),
    )
    const detail: InvoiceView = {
      ...invoiceRow("inv1", "open"),
      subscriptionId: "sub1",
      attemptCount: 1,
      nextAttemptAt: null,
      lineItems: [
        {
          kind: "subscription",
          description: "Pro plan — monthly",
          quantity: 1,
          unitPriceMinor: 500000,
          amountMinor: 500000,
        },
      ],
      paymentAttempts: [
        {
          attemptNumber: 1,
          status: "failed",
          gatewayReference: "ref1",
          failureReason: "card declined",
          createdAt: "2026-04-01T00:00:00Z",
        },
      ],
    }
    vi.mocked(api.billing.getInvoice).mockResolvedValue(ok(detail))
    renderWithClient(<InvoicesPage />)
    fireEvent.click((await screen.findAllByTestId("invoice-row"))[0])
    expect(await screen.findByTestId("invoice-detail")).toBeTruthy()
    expect(screen.getByText("Pro plan — monthly")).toBeTruthy()
    expect(screen.getByText("card declined")).toBeTruthy()
  })

  it("shows a not-found state for a missing invoice", async () => {
    vi.mocked(api.billing.listInvoices).mockResolvedValue(
      ok({ invoices: [invoiceRow("inv1", "open")], total: 1 }),
    )
    vi.mocked(api.billing.getInvoice).mockRejectedValue(
      new ApiError(404, "invoice_not_found", "no such invoice", "/p"),
    )
    renderWithClient(<InvoicesPage />)
    fireEvent.click((await screen.findAllByTestId("invoice-row"))[0])
    expect(await screen.findByTestId("invoice-not-found")).toBeTruthy()
  })
})

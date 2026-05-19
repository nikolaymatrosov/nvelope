// Invoice and payment history with a master/detail view (Phase 5 US4).

import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import type { InvoiceStatus, InvoiceView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isNotFound } from "@/lib/errors"
import { formatDate, formatDateTime } from "@/lib/format"
import { DEFAULT_PAGE_SIZE } from "@/lib/api-types"
import { Money } from "@/components/common/money"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { BillingNav } from "@/components/billing/billing-nav"

export const Route = createFileRoute("/t/$slug/billing/invoices")({
  component: InvoicesPage,
})

const STATUS: Record<
  InvoiceStatus,
  { label: string; variant: "secondary" | "destructive" | "outline" }
> = {
  open: { label: "Unpaid", variant: "destructive" },
  paid: { label: "Paid", variant: "secondary" },
  void: { label: "Void", variant: "outline" },
}

function InvoiceStatusBadge({ status }: { status: InvoiceStatus }) {
  const s = STATUS[status]
  return <Badge variant={s.variant}>{s.label}</Badge>
}

export function InvoicesPage() {
  const { slug } = Route.useParams()
  const [offset, setOffset] = useState(0)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const limit = DEFAULT_PAGE_SIZE

  const query = useQuery({
    queryKey: queryKeys.invoicesPage(slug, limit, offset),
    queryFn: async () =>
      (await api.billing.listInvoices(slug, limit, offset)).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Invoices</h1>
        <p className="text-sm text-muted-foreground">
          Billing history and payment attempts for this workspace.
        </p>
      </header>

      <BillingNav slug={slug} />

      {query.isLoading && (
        <div className="flex flex-col gap-3" data-testid="invoices-loading">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </div>
      )}

      {query.isError && (
        <Empty data-testid="invoices-error" className="border">
          <EmptyHeader>
            <EmptyTitle>Could not load invoices</EmptyTitle>
            <EmptyDescription>{errorMessage(query.error)}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {query.data && query.data.invoices.length === 0 && (
        <Empty data-testid="invoices-empty" className="border">
          <EmptyHeader>
            <EmptyTitle>No invoices yet</EmptyTitle>
            <EmptyDescription>
              Invoices appear here once a billing period has been charged.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {query.data && query.data.invoices.length > 0 && (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Billing period</TableHead>
                <TableHead>Total</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.invoices.map((invoice) => (
                <TableRow
                  key={invoice.id}
                  data-testid="invoice-row"
                  data-status={invoice.status}
                  className="cursor-pointer"
                  onClick={() => setSelectedId(invoice.id)}
                >
                  <TableCell>
                    {formatDate(invoice.periodStart)} –{" "}
                    {formatDate(invoice.periodEnd)}
                  </TableCell>
                  <TableCell>
                    <Money
                      amountMinor={invoice.totalMinor}
                      currency={invoice.currency}
                    />
                  </TableCell>
                  <TableCell>
                    <InvoiceStatusBadge status={invoice.status} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">
              {offset + 1}–{offset + query.data.invoices.length} of{" "}
              {query.data.total}
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - limit))}
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={offset + limit >= query.data.total}
                onClick={() => setOffset(offset + limit)}
              >
                Next
              </Button>
            </div>
          </div>
        </>
      )}

      <InvoiceDetailDialog
        slug={slug}
        invoiceId={selectedId}
        onClose={() => setSelectedId(null)}
      />
    </div>
  )
}

function InvoiceDetailDialog({
  slug,
  invoiceId,
  onClose,
}: {
  slug: string
  invoiceId: string | null
  onClose: () => void
}) {
  const query = useQuery({
    queryKey: queryKeys.invoice(slug, invoiceId ?? ""),
    queryFn: async () =>
      (await api.billing.getInvoice(slug, invoiceId ?? "")).data,
    enabled: invoiceId !== null,
    retry: false,
  })

  const notFound = query.isError && isNotFound(query.error)

  return (
    <Dialog open={invoiceId !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Invoice</DialogTitle>
          <DialogDescription>
            Line items and payment attempts for this invoice.
          </DialogDescription>
        </DialogHeader>

        {query.isLoading && <Skeleton className="h-40 w-full" />}

        {notFound && (
          <p data-testid="invoice-not-found" className="text-sm">
            This invoice could not be found.
          </p>
        )}

        {query.isError && !notFound && (
          <p className="text-sm text-destructive">
            {errorMessage(query.error)}
          </p>
        )}

        {query.data && <InvoiceDetailBody invoice={query.data} />}
      </DialogContent>
    </Dialog>
  )
}

function InvoiceDetailBody({ invoice }: { invoice: InvoiceView }) {
  return (
    <div className="flex flex-col gap-6" data-testid="invoice-detail">
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">
          {formatDate(invoice.periodStart)} – {formatDate(invoice.periodEnd)}
        </span>
        <InvoiceStatusBadge status={invoice.status} />
      </div>

      <section className="flex flex-col gap-2">
        <h3 className="text-sm font-semibold">Line items</h3>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Description</TableHead>
              <TableHead>Qty</TableHead>
              <TableHead>Amount</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {invoice.lineItems.map((li, idx) => (
              <TableRow key={idx}>
                <TableCell>{li.description}</TableCell>
                <TableCell>{li.quantity.toLocaleString()}</TableCell>
                <TableCell>
                  <Money
                    amountMinor={li.amountMinor}
                    currency={invoice.currency}
                  />
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell className="font-semibold">Total</TableCell>
              <TableCell />
              <TableCell className="font-semibold">
                <Money
                  amountMinor={invoice.totalMinor}
                  currency={invoice.currency}
                />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </section>

      <section className="flex flex-col gap-2">
        <h3 className="text-sm font-semibold">Payment attempts</h3>
        {invoice.paymentAttempts.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No payment attempts have been made yet.
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {invoice.paymentAttempts.map((a) => (
              <li
                key={a.attemptNumber}
                className="flex items-center justify-between rounded-lg border p-3 text-sm"
              >
                <div className="flex flex-col">
                  <span className="font-medium">
                    Attempt {a.attemptNumber} —{" "}
                    {a.status === "succeeded" ? "Succeeded" : "Failed"}
                  </span>
                  {a.status === "failed" && a.failureReason && (
                    <span className="text-muted-foreground">
                      {a.failureReason}
                    </span>
                  )}
                </div>
                <span className="text-muted-foreground">
                  {formatDateTime(a.createdAt)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}

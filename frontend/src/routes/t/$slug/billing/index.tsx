// Billing overview (Phase 5 US1, US5): the current plan and subscription state,
// past-due / suspended / pending warnings, the no-subscription state, and the
// settle-balance recovery action.

import { Link, createFileRoute } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CreditCardIcon } from "lucide-react"
import { toast } from "sonner"
import type { ReactNode } from "react"
import type {
  InvoiceSummary,
  SubscriptionResponse,
} from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { ApiError, errorMessage, isNotFound } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  BillingNav,
  SubscriptionStateBadge,
} from "@/components/billing/billing-nav"

export const Route = createFileRoute("/t/$slug/billing/")({
  component: BillingOverview,
})

export function BillingOverview() {
  const { slug } = Route.useParams()
  const { can } = usePermissions(slug)
  const canManage = can("billing:manage")

  const query = useQuery({
    queryKey: queryKeys.subscription(slug),
    queryFn: async () => (await api.billing.getSubscription(slug)).data,
    retry: false,
  })

  const noSubscription =
    query.isError &&
    isNotFound(query.error) &&
    query.error.slug === "no_subscription"

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Billing</h1>
        <p className="text-sm text-muted-foreground">
          Your plan, subscription status, and account balance.
        </p>
      </header>

      <BillingNav slug={slug} />

      {query.isLoading && (
        <div className="flex flex-col gap-3" data-testid="billing-loading">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-32 w-full" />
        </div>
      )}

      {noSubscription && <NoSubscription slug={slug} />}

      {query.isError && !noSubscription && (
        <Empty data-testid="billing-error" className="border">
          <EmptyHeader>
            <EmptyTitle>Could not load billing</EmptyTitle>
            <EmptyDescription>{errorMessage(query.error)}</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button variant="outline" size="sm" onClick={() => query.refetch()}>
              Try again
            </Button>
          </EmptyContent>
        </Empty>
      )}

      {query.data && (
        <SubscriptionPanel
          slug={slug}
          data={query.data}
          canManage={canManage}
        />
      )}
    </div>
  )
}

function NoSubscription({ slug }: { slug: string }) {
  return (
    <Empty data-testid="billing-no-subscription" className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <CreditCardIcon />
        </EmptyMedia>
        <EmptyTitle>No active subscription</EmptyTitle>
        <EmptyDescription>
          This workspace is not subscribed to a plan. Choose a plan to start
          sending.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button asChild>
          <Link to="/t/$slug/billing/plans" params={{ slug }}>
            Browse plans
          </Link>
        </Button>
      </EmptyContent>
    </Empty>
  )
}

function SubscriptionPanel({
  slug,
  data,
  canManage,
}: {
  slug: string
  data: SubscriptionResponse
  canManage: boolean
}) {
  const { subscription, usage } = data
  const { state } = subscription

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <h2 className="text-lg font-semibold">{subscription.plan.name}</h2>
        <SubscriptionStateBadge state={state} />
      </div>

      {state === "pending" && (
        <Alert data-testid="billing-pending">
          <AlertTitle>Subscription pending</AlertTitle>
          <AlertDescription>
            The first payment for this subscription is being processed. This
            page will reflect the result once the charge resolves.
          </AlertDescription>
        </Alert>
      )}

      {state === "past_due" && (
        <Alert variant="destructive" data-testid="billing-past-due">
          <AlertTitle>Payment failed — action needed</AlertTitle>
          <AlertDescription>
            A payment for this subscription was declined and is being retried
            automatically. Settle the outstanding balance below to avoid the
            account being suspended.
          </AlertDescription>
        </Alert>
      )}

      {state === "suspended" && (
        <Alert variant="destructive" data-testid="billing-suspended">
          <AlertTitle>Account suspended for non-payment</AlertTitle>
          <AlertDescription>
            Sending is disabled because the outstanding balance was not
            settled. Settle it below to reinstate the account and re-enable
            sending.
          </AlertDescription>
        </Alert>
      )}

      {(state === "past_due" || state === "suspended") && (
        <SettleOutstanding slug={slug} canManage={canManage} />
      )}

      <Card>
        <CardHeader>
          <CardTitle>Current plan</CardTitle>
          <CardDescription>
            Sends over the allowance are{" "}
            {subscription.plan.overageMode === "block"
              ? "blocked"
              : "billed as overage"}
            .
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm sm:grid-cols-2">
          <Field label="Plan" value={subscription.plan.name} />
          <Field
            label="Billing period"
            value={`${formatDate(subscription.currentPeriodStart)} – ${formatDate(
              subscription.currentPeriodEnd,
            )}`}
          />
          <Field
            label="Usage this period"
            value={
              <Link
                to="/t/$slug/billing/usage"
                params={{ slug }}
                className="text-primary hover:underline"
              >
                {usage.usedSends.toLocaleString()} of{" "}
                {usage.includedSends.toLocaleString()} sends
              </Link>
            }
          />
          {subscription.cancelAtPeriodEnd && (
            <Field
              label="Renewal"
              value="Cancels at the end of the current period"
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Field({
  label,
  value,
}: {
  label: string
  value: ReactNode
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <span>{value}</span>
    </div>
  )
}

// SettleOutstanding finds the tenant's open invoice and offers a settle action
// that re-charges it through the gateway (US5).
function SettleOutstanding({
  slug,
  canManage,
}: {
  slug: string
  canManage: boolean
}) {
  const queryClient = useQueryClient()

  const invoicesQuery = useQuery({
    queryKey: queryKeys.invoices(slug),
    queryFn: async () =>
      (await api.billing.listInvoices(slug, 50, 0)).data.invoices,
    retry: false,
  })

  const openInvoice: InvoiceSummary | undefined = (
    invoicesQuery.data ?? []
  ).find((i) => i.status === "open")

  const settle = useMutation({
    mutationFn: () =>
      api.billing.settleInvoice(slug, openInvoice?.id ?? ""),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.subscription(slug),
      })
      await queryClient.invalidateQueries({
        queryKey: queryKeys.invoices(slug),
      })
      toast.success("Payment received — your account has been reinstated.")
    },
    onError: async (e) => {
      if (e instanceof ApiError && e.slug === "payment_failed") {
        toast.error(
          "The payment was declined again. The account remains suspended — please try once more or use a different payment method.",
        )
        return
      }
      if (e instanceof ApiError && e.slug === "invoice_not_settleable") {
        await queryClient.invalidateQueries({
          queryKey: queryKeys.invoices(slug),
        })
        toast.message("That invoice has already been settled.")
        return
      }
      toast.error(errorMessage(e))
    },
  })

  if (!openInvoice) return null

  return (
    <Card data-testid="settle-panel">
      <CardHeader>
        <CardTitle>Outstanding balance</CardTitle>
        <CardDescription>
          Settle the unpaid invoice to bring the account back to good standing.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-4">
        <Link
          to="/t/$slug/billing/invoices"
          params={{ slug }}
          className="text-sm text-primary hover:underline"
        >
          View unpaid invoice
        </Link>
        {canManage && (
          <Button
            disabled={settle.isPending}
            onClick={() => settle.mutate()}
          >
            {settle.isPending ? "Charging…" : "Settle balance now"}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

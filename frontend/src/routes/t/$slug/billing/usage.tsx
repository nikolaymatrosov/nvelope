// Current-period usage against the plan allowance (Phase 5 US3).

import { Link, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { InfoIcon } from "lucide-react"
import type { SubscriptionResponse } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isNotFound } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { UsageMeter } from "@/components/common/usage-meter"
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
  EmptyTitle,
} from "@/components/ui/empty"
import { BillingNav } from "@/components/billing/billing-nav"

export const Route = createFileRoute("/t/$slug/billing/usage")({
  component: UsagePage,
})

export function UsagePage() {
  const { slug } = Route.useParams()

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
        <h1 className="text-2xl font-semibold">Usage</h1>
        <p className="text-sm text-muted-foreground">
          Metered sends consumed in the current billing period.
        </p>
      </header>

      <BillingNav slug={slug} />

      {query.isLoading && (
        <div className="flex flex-col gap-3" data-testid="usage-loading">
          <Skeleton className="h-32 w-full" />
        </div>
      )}

      {noSubscription && (
        <Empty data-testid="usage-no-subscription" className="border">
          <EmptyHeader>
            <EmptyTitle>No active subscription</EmptyTitle>
            <EmptyDescription>
              Subscribe to a plan to start metering usage.
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
      )}

      {query.isError && !noSubscription && (
        <Empty data-testid="usage-error" className="border">
          <EmptyHeader>
            <EmptyTitle>Could not load usage</EmptyTitle>
            <EmptyDescription>{errorMessage(query.error)}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {query.data && <UsageBody slug={slug} data={query.data} />}
    </div>
  )
}

function UsageBody({
  slug,
  data,
}: {
  slug: string
  data: SubscriptionResponse
}) {
  const { subscription, usage } = data
  const blockMode = subscription.plan.overageMode === "block"
  const exhausted =
    usage.includedSends > 0 && usage.remainingSends <= 0

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>This period</CardTitle>
          <CardDescription>
            {formatDate(subscription.currentPeriodStart)} –{" "}
            {formatDate(subscription.currentPeriodEnd)}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <UsageMeter
            used={usage.usedSends}
            included={usage.includedSends}
          />

          {exhausted && blockMode && (
            <Alert variant="destructive" data-testid="usage-blocked">
              <AlertTitle>Allowance reached</AlertTitle>
              <AlertDescription>
                This plan blocks sends beyond the included allowance. Further
                campaign starts and transactional sends will be rejected until
                the next billing period begins.
              </AlertDescription>
            </Alert>
          )}

          {!blockMode && (
            <Alert data-testid="usage-overage">
              <AlertTitle>Overage billing</AlertTitle>
              <AlertDescription>
                Sends beyond the allowance are billed as overage.{" "}
                {usage.overageSends.toLocaleString()} overage send
                {usage.overageSends === 1 ? "" : "s"} recorded so far this
                period.
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      <Alert data-testid="usage-refresh-note">
        <InfoIcon />
        <AlertTitle>Figures update periodically</AlertTitle>
        <AlertDescription>
          Usage is rolled up by a background process, so sends from the last
          few minutes may not yet be reflected here.
        </AlertDescription>
      </Alert>

      <p className="text-sm text-muted-foreground">
        Usage for closed periods is reflected in each period’s{" "}
        <Link
          to="/t/$slug/billing/invoices"
          params={{ slug }}
          className="text-primary hover:underline"
        >
          invoice
        </Link>
        .
      </p>
    </div>
  )
}

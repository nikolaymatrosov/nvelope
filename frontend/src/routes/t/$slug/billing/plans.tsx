// Plan catalogue and self-service subscribe flow (Phase 5 US2).

import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import type { PlanView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { ApiError, errorMessage, isNotFound } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Money, formatMoney } from "@/components/common/money"
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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

export const Route = createFileRoute("/t/$slug/billing/plans")({
  component: PlansPage,
})

export function PlansPage() {
  const { slug } = Route.useParams()
  const { t } = useTranslation(["billing", "common"])
  const { can } = usePermissions(slug)
  const canManage = can("billing:manage")
  const queryClient = useQueryClient()

  const [selected, setSelected] = useState<PlanView | null>(null)
  const [declined, setDeclined] = useState(false)

  const plansQuery = useQuery({
    queryKey: queryKeys.plans(slug),
    queryFn: async () => (await api.billing.plans(slug)).data.plans,
  })

  const subQuery = useQuery({
    queryKey: queryKeys.subscription(slug),
    queryFn: async () => (await api.billing.getSubscription(slug)).data,
    retry: false,
  })

  // A 404 means there is no subscription; any success means one exists unless
  // it has been cancelled.
  const noSubscription =
    subQuery.isError &&
    isNotFound(subQuery.error) &&
    subQuery.error.slug === "no_subscription"
  const alreadySubscribed =
    subQuery.isSuccess &&
    subQuery.data.subscription.state !== "cancelled"
  const canSubscribe = canManage && (noSubscription || subQuery.isSuccess) &&
    !alreadySubscribed

  const subscribe = useMutation({
    mutationFn: (planId: string) => api.billing.subscribe(slug, planId),
    onSuccess: async () => {
      setSelected(null)
      setDeclined(false)
      await queryClient.invalidateQueries({
        queryKey: queryKeys.subscription(slug),
      })
      toast.success(t("plans.toast.subscribed"))
    },
    onError: (e) => {
      setSelected(null)
      if (e instanceof ApiError && e.slug === "payment_failed") {
        setDeclined(true)
        return
      }
      if (e instanceof ApiError && e.slug === "subscription_exists") {
        toast.error(t("plans.toast.subscriptionExists"))
        return
      }
      toast.error(errorMessage(e))
    },
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("plans.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("plans.description")}
        </p>
      </header>

      <BillingNav slug={slug} />

      {alreadySubscribed && (
        <Alert data-testid="plans-already-subscribed">
          <AlertTitle>{t("plans.alreadySubscribed.title")}</AlertTitle>
          <AlertDescription>
            {t("plans.alreadySubscribed.descriptionPrefix")}
            <Link
              to="/t/$slug/billing"
              params={{ slug }}
              className="text-primary hover:underline"
            >
              {t("plans.alreadySubscribed.link")}
            </Link>
            {t("plans.alreadySubscribed.descriptionSuffix")}
          </AlertDescription>
        </Alert>
      )}

      {declined && (
        <Alert variant="destructive" data-testid="plans-declined">
          <AlertTitle>{t("plans.declined.title")}</AlertTitle>
          <AlertDescription>
            {t("plans.declined.description")}
          </AlertDescription>
        </Alert>
      )}

      {plansQuery.isLoading && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-48 w-full" />
          <Skeleton className="h-48 w-full" />
          <Skeleton className="h-48 w-full" />
        </div>
      )}

      {plansQuery.isError && (
        <Empty data-testid="plans-error" className="border">
          <EmptyHeader>
            <EmptyTitle>{t("plans.loadError.title")}</EmptyTitle>
            <EmptyDescription>
              {errorMessage(plansQuery.error)}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {plansQuery.data && plansQuery.data.length === 0 && (
        <Empty data-testid="plans-empty" className="border">
          <EmptyHeader>
            <EmptyTitle>{t("plans.empty.title")}</EmptyTitle>
            <EmptyDescription>
              {t("plans.empty.description")}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {plansQuery.data && plansQuery.data.length > 0 && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {plansQuery.data.map((plan) => (
            <PlanCard
              key={plan.id}
              plan={plan}
              canSubscribe={canSubscribe}
              onSubscribe={() => {
                setDeclined(false)
                setSelected(plan)
              }}
            />
          ))}
        </div>
      )}

      <Dialog
        open={selected !== null}
        onOpenChange={(o) => !o && setSelected(null)}
      >
        <DialogContent>
          {selected && (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t("plans.dialog.title", { plan: selected.name })}
                </DialogTitle>
                <DialogDescription>
                  {t("plans.dialog.description", {
                    price: formatMoney(
                      selected.priceMinor,
                      selected.currency,
                    ),
                    period: selected.billingPeriod,
                  })}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setSelected(null)}
                  disabled={subscribe.isPending}
                >
                  {t("common:actions.cancel")}
                </Button>
                <Button
                  type="button"
                  disabled={subscribe.isPending}
                  onClick={() => subscribe.mutate(selected.id)}
                >
                  {subscribe.isPending
                    ? t("plans.dialog.processing")
                    : t("plans.dialog.confirm")}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

function PlanCard({
  plan,
  canSubscribe,
  onSubscribe,
}: {
  plan: PlanView
  canSubscribe: boolean
  onSubscribe: () => void
}) {
  const { t } = useTranslation("billing")
  return (
    <Card className="flex flex-col">
      <CardHeader>
        <CardTitle>{plan.name}</CardTitle>
        <CardDescription>
          <span className="text-xl font-semibold text-foreground">
            <Money amountMinor={plan.priceMinor} currency={plan.currency} />
          </span>{" "}
          {t("plans.card.perPeriod", { period: plan.billingPeriod })}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-3 text-sm">
        <ul className="flex flex-col gap-1 text-muted-foreground">
          <li>
            {t("plans.card.includedSends", {
              sends: plan.includedSends.toLocaleString(),
            })}
          </li>
          <li>
            {plan.overageMode === "block"
              ? t("plans.card.overageBlocked")
              : t("plans.card.overageBilled", {
                  price: formatMoney(
                    plan.overagePriceMinor,
                    plan.currency,
                  ),
                })}
          </li>
        </ul>
        <div className="mt-auto">
          <Button
            className="w-full"
            disabled={!canSubscribe}
            onClick={onSubscribe}
          >
            {t("plans.card.subscribe")}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

// Shared chrome for the billing area (Phase 5 UI): the sub-navigation between
// the billing pages and the subscription-state badge.

import { Link } from "@tanstack/react-router"
import type { SubscriptionState } from "@/lib/api-types"
import { Badge } from "@/components/ui/badge"

const TABS = [
  { label: "Overview", to: "/t/$slug/billing" },
  { label: "Plans", to: "/t/$slug/billing/plans" },
  { label: "Usage", to: "/t/$slug/billing/usage" },
  { label: "Invoices", to: "/t/$slug/billing/invoices" },
] as const

export function BillingNav({ slug }: { slug: string }) {
  return (
    <nav className="flex gap-1 border-b">
      {TABS.map((tab) => (
        <Link
          key={tab.to}
          to={tab.to}
          params={{ slug }}
          activeOptions={{ exact: tab.to === "/t/$slug/billing" }}
          className="border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground [&.active]:border-primary [&.active]:text-foreground"
        >
          {tab.label}
        </Link>
      ))}
    </nav>
  )
}

const STATE_LABEL: Record<SubscriptionState, string> = {
  pending: "Pending",
  active: "Active",
  past_due: "Past due",
  suspended: "Suspended",
  cancelled: "Cancelled",
}

const STATE_VARIANT: Record<
  SubscriptionState,
  "default" | "secondary" | "destructive" | "outline"
> = {
  pending: "secondary",
  active: "default",
  past_due: "destructive",
  suspended: "destructive",
  cancelled: "outline",
}

export function SubscriptionStateBadge({
  state,
}: {
  state: SubscriptionState
}) {
  return <Badge variant={STATE_VARIANT[state]}>{STATE_LABEL[state]}</Badge>
}

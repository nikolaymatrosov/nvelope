import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { expect, within } from "storybook/test"
import { BillingNav, SubscriptionStateBadge } from "./billing-nav"
import type { SubscriptionState } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

const SLUG = "acme"

// BillingNav renders TanStack Router <Link>s with typed `to` route ids, so it
// needs a router whose tree registers each billing sub-route. A minimal
// in-memory router resolves the links and applies the active-tab styling
// (the `[&.active]` classes) without the full app route tree.
function billingRouter(initialPath: string) {
  const rootRoute = createRootRoute({
    component: () => <BillingNav slug={SLUG} />,
  })
  const stub = (path: string) =>
    createRoute({ getParentRoute: () => rootRoute, path, component: () => null })
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      stub("/t/$slug/billing"),
      stub("/t/$slug/billing/plans"),
      stub("/t/$slug/billing/usage"),
      stub("/t/$slug/billing/invoices"),
    ]),
    history: createMemoryHistory({ initialEntries: [initialPath] }),
    context: {},
  })
  return <RouterProvider router={router} />
}

const meta = {
  component: BillingNav,
  tags: ["ai-generated"],
  args: { slug: SLUG },
} satisfies Meta<typeof BillingNav>

export default meta
type Story = StoryObj<typeof meta>

// Landing on the Overview route → that tab carries the active styling
// (exact-match), the rest are inactive links.
export const OverviewActive: Story = {
  render: () => billingRouter(`/t/${SLUG}/billing`),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByRole("link", { name: "Overview" })).toBeVisible()
    await expect(canvas.getByRole("link", { name: "Invoices" })).toBeVisible()
  },
}

// On a deeper billing route, Overview is no longer exact-active but its
// sibling tab is — exercises the per-tab `activeOptions.exact` branch.
export const InvoicesActive: Story = {
  render: () => billingRouter(`/t/${SLUG}/billing/invoices`),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    const invoices = canvas.getByRole("link", { name: "Invoices" })
    await expect(invoices).toHaveClass("active")
  },
}

// ── SubscriptionStateBadge (the file's other export) ──────────────────────────
// Purely presentational: each subscription state maps to a label + Badge
// variant. Rendered directly (no router needed).

function badgeStory(state: SubscriptionState): Story {
  return { render: () => <SubscriptionStateBadge state={state} /> }
}

export const BadgeActive = badgeStory("active")
export const BadgePending = badgeStory("pending")
export const BadgePastDue = badgeStory("past_due")
export const BadgeSuspended = badgeStory("suspended")
export const BadgeCancelled = badgeStory("cancelled")

// Every state at once — proves the full label map renders.
export const BadgeAllStates: Story = {
  render: () => {
    const states: Array<SubscriptionState> = [
      "pending",
      "active",
      "past_due",
      "suspended",
      "cancelled",
    ]
    return (
      <div className="flex flex-wrap gap-2">
        {states.map((s) => (
          <SubscriptionStateBadge key={s} state={s} />
        ))}
      </div>
    )
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByText("Past due")).toBeInTheDocument()
    await expect(canvas.getByText("Suspended")).toBeInTheDocument()
  },
}

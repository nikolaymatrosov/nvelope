import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { expect, within } from "storybook/test"
import { SuspensionBanner } from "./suspension-banner"
import type {
  SubscriptionResponse,
  SubscriptionState,
} from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

function subscription(state: SubscriptionState): SubscriptionResponse {
  return {
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
      usedSends: 0,
      overageSends: 0,
      remainingSends: 10000,
    },
  }
}

// The banner renders a TanStack Router <Link> when suspended, so a real router
// must wrap it. A minimal in-memory router with the routes the component links
// to ("/t/$slug/billing") is enough; we mount the banner inside the index route
// so it renders within router context.
function harness(state: SubscriptionState | "none") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  if (state !== "none") {
    // Seed the exact key the component reads so useQuery resolves synchronously
    // from cache and never hits the network.
    client.setQueryData(queryKeys.subscription("acme"), subscription(state))
  } else {
    client.setQueryData(queryKeys.subscription("acme"), null)
  }

  const rootRoute = createRootRoute({
    component: () => <SuspensionBanner slug="acme" />,
  })
  const billingRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/t/$slug/billing",
    component: () => null,
  })
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => null,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([billingRoute, indexRoute]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
    context: {},
  })

  return (
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

const meta = {
  component: SuspensionBanner,
  tags: ["ai-generated"],
  args: { slug: "acme" },
} satisfies Meta<typeof SuspensionBanner>

export default meta
type Story = StoryObj<typeof meta>

// Suspended subscription → the warning banner with a settle-balance link.
export const Suspended: Story = {
  render: () => harness("suspended"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      await canvas.findByTestId("suspension-banner"),
    ).toBeInTheDocument()
  },
}

// Active subscription → renders nothing.
export const Active: Story = {
  render: () => harness("active"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.queryByTestId("suspension-banner")).toBeNull()
  },
}

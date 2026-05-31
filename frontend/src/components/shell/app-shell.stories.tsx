import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { expect, within } from "storybook/test"
import { AppShell } from "./app-shell"
import type {
  PlatformAccount,
  SubscriptionResponse,
  WorkspaceInfo,
} from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

const SLUG = "acme"

const account: PlatformAccount = {
  user: { id: "u1", name: "Ada Lovelace", email: "ada@example.com", locale: "en" },
  tenants: [],
}

const ownerWorkspace: WorkspaceInfo = {
  tenant: { name: "Acme Inc" },
  members: [
    { user_id: "u1", email: "ada@example.com", name: "Ada", role: "owner" },
  ],
}

function subscription(
  state: SubscriptionResponse["subscription"]["state"],
): SubscriptionResponse {
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

const SEGMENTS = [
  "dashboard",
  "subscribers",
  "lists",
  "sending-domains",
  "templates",
  "campaigns",
  "suppressions",
  "transactional",
  "access",
  "import-export",
  "audit",
  "billing",
  "public-pages",
  "branding",
  "media",
  "settings",
]

// AppShell is the most router-coupled component: it composes the sidebar,
// top bar, and suspension banner, each of which independently needs the router
// + query cache + sidebar context. The harness seeds every query those children
// read (me, tenant, roles, subscription) and registers every nav target as a
// stub route so the typed Links resolve.
function harness(
  subState: SubscriptionResponse["subscription"]["state"],
  children: React.ReactNode,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(queryKeys.me(), account)
  client.setQueryData(queryKeys.tenant(SLUG), ownerWorkspace)
  client.setQueryData(queryKeys.roles(SLUG), [])
  client.setQueryData(queryKeys.subscription(SLUG), subscription(subState))

  const rootRoute = createRootRoute({
    component: () => (
      <AppShell slug={SLUG} workspaceName="Acme Inc">
        {children}
      </AppShell>
    ),
  })
  const stub = (path: string) =>
    createRoute({ getParentRoute: () => rootRoute, path, component: () => null })
  const routeTree = rootRoute.addChildren([
    stub("/"),
    stub("/login"),
    stub("/account"),
    stub("/t/$slug"),
    ...SEGMENTS.map((seg) => stub(`/t/${SLUG}/${seg}`)),
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [`/t/${SLUG}/dashboard`] }),
    context: {},
  })

  return (
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

const meta = {
  component: AppShell,
  tags: ["ai-generated"],
  args: { slug: SLUG, workspaceName: "Acme Inc", children: null },
} satisfies Meta<typeof AppShell>

export default meta
type Story = StoryObj<typeof meta>

// The full workspace chrome: sidebar nav, top bar, and the page content.
export const Default: Story = {
  render: () =>
    harness(
      "active",
      <div data-testid="shell-content">
        <h1 className="text-xl font-semibold">Dashboard</h1>
        <p className="text-muted-foreground">Welcome back, Ada.</p>
      </div>,
    ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(await canvas.findByTestId("shell-content")).toBeInTheDocument()
    await expect(canvas.getByText("Campaigns")).toBeInTheDocument()
    // Active subscription → no suspension banner.
    await expect(canvas.queryByTestId("suspension-banner")).toBeNull()
  },
}

// A suspended workspace surfaces the warning banner above the page content.
export const Suspended: Story = {
  render: () =>
    harness("suspended", <div data-testid="shell-content">Dashboard</div>),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      await canvas.findByTestId("suspension-banner"),
    ).toBeInTheDocument()
  },
}

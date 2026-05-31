import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { expect, within } from "storybook/test"
import { WorkspaceSidebar } from "./sidebar"
import type { PlatformAccount, WorkspaceInfo } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"
import { SidebarProvider } from "@/components/ui/sidebar"

const SLUG = "acme"

const account: PlatformAccount = {
  user: { id: "u1", name: "Ada", email: "ada@example.com", locale: "en" },
  tenants: [],
}

// An owner sees every nav entry (derivePermissions short-circuits to the full
// permission set for the `owner` role), which keeps the harness data minimal:
// no roles query is consulted.
const ownerWorkspace: WorkspaceInfo = {
  tenant: { name: "Acme Inc" },
  members: [
    { user_id: "u1", email: "ada@example.com", name: "Ada", role: "owner" },
  ],
}

// A member whose role grants nothing collapses the nav to the always-visible
// entry (Access has empty `requires`).
const memberWorkspace: WorkspaceInfo = {
  tenant: { name: "Acme Inc" },
  members: [
    { user_id: "u1", email: "ada@example.com", name: "Ada", role: "viewer" },
  ],
}

// WorkspaceSidebar needs the router (Link / useLocation), usePermissions
// (me() + tenant() queries), useTranslation, and SidebarProvider. The router
// registers every nav target as a stub route so the typed Links resolve.
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

function harness(workspace: WorkspaceInfo) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(queryKeys.me(), account)
  client.setQueryData(queryKeys.tenant(SLUG), workspace)
  // Seed roles as empty so the non-owner case resolves to no permissions
  // without a network call.
  client.setQueryData(queryKeys.roles(SLUG), [])

  const rootRoute = createRootRoute({
    component: () => (
      <SidebarProvider>
        <WorkspaceSidebar slug={SLUG} />
      </SidebarProvider>
    ),
  })
  const stub = (path: string) =>
    createRoute({ getParentRoute: () => rootRoute, path, component: () => null })
  const children = [
    stub("/"),
    stub("/t/$slug"),
    ...SEGMENTS.map((seg) => stub(`/t/${SLUG}/${seg}`)),
  ]
  const router = createRouter({
    routeTree: rootRoute.addChildren(children),
    history: createMemoryHistory({
      initialEntries: [`/t/${SLUG}/dashboard`],
    }),
    context: {},
  })

  return (
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

const meta = {
  component: WorkspaceSidebar,
  tags: ["ai-generated"],
  args: { slug: SLUG },
} satisfies Meta<typeof WorkspaceSidebar>

export default meta
type Story = StoryObj<typeof meta>

// An owner: the full navigation is visible.
export const Owner: Story = {
  render: () => harness(ownerWorkspace),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(await canvas.findByText("Campaigns")).toBeInTheDocument()
    await expect(canvas.getByText("Billing")).toBeInTheDocument()
  },
}

// A member with no role permissions: only the always-visible People & Access
// entry shows (its `requires` is empty).
export const MemberNoPermissions: Story = {
  render: () => harness(memberWorkspace),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(await canvas.findByText("People & Access")).toBeInTheDocument()
    await expect(canvas.queryByText("Campaigns")).toBeNull()
  },
}

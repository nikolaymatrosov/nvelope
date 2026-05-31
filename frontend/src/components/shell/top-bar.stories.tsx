import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { expect, within } from "storybook/test"
import { TopBar } from "./top-bar"
import type { PlatformAccount } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"
import { SidebarProvider } from "@/components/ui/sidebar"

const account: PlatformAccount = {
  user: {
    id: "u1",
    name: "Ada Lovelace",
    email: "ada@example.com",
    locale: "en",
  },
  tenants: [],
}

// TopBar pulls in useNavigate (router), useSession (me() query), and
// SidebarTrigger (SidebarProvider). The harness supplies all three: a minimal
// in-memory router with the routes the menu navigates to, a QueryClient seeded
// with the signed-in account, and the sidebar context.
function harness(workspaceName: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(queryKeys.me(), account)

  const rootRoute = createRootRoute({
    component: () => (
      <SidebarProvider>
        <TopBar workspaceName={workspaceName} />
      </SidebarProvider>
    ),
  })
  const stub = (path: string) =>
    createRoute({ getParentRoute: () => rootRoute, path, component: () => null })
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      stub("/"),
      stub("/login"),
      stub("/account"),
    ]),
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
  component: TopBar,
  tags: ["ai-generated"],
  args: { workspaceName: "Acme Inc" },
} satisfies Meta<typeof TopBar>

export default meta
type Story = StoryObj<typeof meta>

// The bar shows the workspace name and the account avatar with initials.
export const Default: Story = {
  render: (args) => harness(args.workspaceName),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByText("Acme Inc")).toBeInTheDocument()
    await expect(canvas.getByText("AL")).toBeInTheDocument()
  },
}

// Opening the account menu (a portal) reveals the user details and actions.
export const AccountMenuOpen: Story = {
  render: (args) => harness(args.workspaceName),
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("button", { name: /account/i }))
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("ada@example.com")).toBeVisible()
  },
}

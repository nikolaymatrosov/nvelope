import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from "@tanstack/react-router"
import { fn } from "storybook/test"
import { EditorScreen } from "./EditorScreen"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: EditorScreen,
  parameters: { layout: "fullscreen" },
  // useBlocker needs a router; the branding query needs a query client.
  decorators: [
    (Story) => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const router = createRouter({
        routeTree: createRootRoute({ component: () => <Story /> }),
        history: createMemoryHistory({ initialEntries: ["/"] }),
        context: {},
      })
      return (
        <QueryClientProvider client={client}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      )
    },
  ],
  args: {
    slug: "acme",
    section: "Templates",
    row: {
      id: "tpl-1",
      name: "Welcome email",
      body_doc: {
        version: 1,
        type: "doc",
        content: [
          { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "Hello!" }] },
          { type: "paragraph", content: [{ type: "text", text: "Thanks for joining." }] },
          { type: "button", attrs: { label: "Get started", href: "https://example.com" } },
        ],
      },
      body_html: "",
      theme: null,
      updated_at: "2026-01-01T00:00:00Z",
    },
    editable: true,
    onBack: fn(),
    convert: fn(),
    save: fn(() => Promise.resolve({ data: { updatedAt: "2026-01-02T00:00:00Z", warnings: [] } })),
    refetch: fn(),
    onSaved: fn(async () => {}),
  },
} satisfies Meta<typeof EditorScreen>

export default meta
type Story = StoryObj<typeof meta>

export const Editing: Story = {}
export const ReadOnly: Story = { args: { editable: false } }

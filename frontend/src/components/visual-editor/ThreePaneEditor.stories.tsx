import { useState } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, within } from "storybook/test"
import { ThreePaneEditor } from "./ThreePaneEditor"
import type { BrandingView, MergeTagsResponse, VisualDoc } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

const SLUG = "acme"

const sampleDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [
    { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "Spring sale" }] },
    { type: "paragraph", content: [{ type: "text", text: "Up to 40% off this week only." }] },
    { type: "button", attrs: { label: "Shop now", href: "https://example.test/shop" } },
  ],
}

const branding: BrandingView = { logo_url: "", primary_color: "#0066cc", custom_css: "" }
const mergeTags: MergeTagsResponse = {
  subscriber: [{ slug: "first_name", displayName: "First name", type: "text", builtIn: true }],
  campaign: [{ key: "subject", displayName: "Subject" }],
}

// Seed the queries the embedded VisualEmailEditor reads (branding via
// useEditorTheme, merge tags via the picker) so the editor mounts without a
// round-trip — the same pattern VisualEmailEditor.stories uses.
function makeClient() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(queryKeys.branding(SLUG), branding)
  client.setQueryData(queryKeys.mergeTags(SLUG), mergeTags)
  return client
}

function Harness() {
  const [value, setValue] = useState<VisualDoc>(sampleDoc)
  return (
    <div style={{ height: 520, border: "1px solid #e5e7eb" }}>
      <ThreePaneEditor slug={SLUG} value={value} onChange={setValue} />
    </div>
  )
}

const meta = {
  component: ThreePaneEditor,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <QueryClientProvider client={makeClient()}>
        <Story />
      </QueryClientProvider>
    ),
  ],
} satisfies Meta<typeof ThreePaneEditor>

export default meta
type Story = StoryObj<typeof meta>

export const ThreePane: Story = {
  render: () => <Harness />,
}

export const HasCollapseToggles: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(await canvas.findByTestId("ve-three-pane")).toBeInTheDocument()
    await expect(canvas.getByTestId("ve-toggle-left")).toBeInTheDocument()
    await expect(canvas.getByTestId("ve-toggle-right")).toBeInTheDocument()
  },
}

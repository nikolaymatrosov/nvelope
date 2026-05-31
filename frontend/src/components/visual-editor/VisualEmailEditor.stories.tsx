import { useState } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, within } from "storybook/test"
import { VisualEmailEditor } from "./VisualEmailEditor"
import type {
  BrandingView,
  MergeTagsResponse,
  VisualDoc,
} from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

const SLUG = "acme"

const value: VisualDoc = {
  version: 1,
  type: "doc",
  content: [
    {
      type: "heading",
      attrs: { level: 1 },
      content: [{ type: "text", text: "Welcome aboard" }],
    },
    {
      type: "paragraph",
      content: [
        { type: "text", text: "Type " },
        { type: "text", marks: [{ type: "bold" }], text: "/" },
        { type: "text", text: " to insert a block." },
      ],
    },
  ],
}

const branding: BrandingView = {
  logo_url: "",
  primary_color: "#0066cc",
  custom_css: "",
}

const mergeTags: MergeTagsResponse = {
  subscriber: [
    {
      slug: "first_name",
      displayName: "First name",
      type: "text",
      builtIn: true,
    },
  ],
  campaign: [{ key: "subject", displayName: "Subject" }],
}

// Seed every query the editor + its children read synchronously so the
// composition mounts without any BFF/Go round-trip:
//   - useEditorTheme → queryKeys.branding(slug)
//   - MergeTagPicker → queryKeys.mergeTags(slug)
// (The MediaPicker query is `enabled: open` and stays idle while no image
// pick is in flight.)
function makeClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(queryKeys.branding(SLUG), branding)
  client.setQueryData(queryKeys.mergeTags(SLUG), mergeTags)
  return client
}

// Controlled wrapper — the editor owns the canonical VisualDoc and emits every
// change via onChange, exactly like the real campaign/template routes.
function Harness() {
  const [doc, setDoc] = useState<VisualDoc>(value)
  return (
    <div style={{ width: 640 }}>
      <VisualEmailEditor slug={SLUG} value={doc} onChange={setDoc} />
    </div>
  )
}

const meta = {
  component: VisualEmailEditor,
  tags: ["ai-generated"],
  args: { slug: SLUG, value, onChange: () => {} },
  decorators: [
    (Story) => (
      <QueryClientProvider client={makeClient()}>
        <Story />
      </QueryClientProvider>
    ),
  ],
  render: () => <Harness />,
} satisfies Meta<typeof VisualEmailEditor>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

// The editor mounts with the seeded doc rendered into the ProseMirror surface.
export const RendersSeededDoc: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    const editor = await canvas.findByTestId("ve-editor")
    await expect(editor).toBeInTheDocument()
    await expect(editor).toHaveTextContent("Welcome aboard")
  },
}

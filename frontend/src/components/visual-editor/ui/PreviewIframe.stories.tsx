import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, userEvent, within } from "storybook/test"
import { PreviewIframe } from "./PreviewIframe"
import type {
  RenderPreviewResponse,
  Theme,
  VisualDoc,
} from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

const SLUG = "acme"

const doc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [
    {
      type: "paragraph",
      content: [{ type: "text", text: "Hello from the preview." }],
    },
  ],
}

const theme: Theme = {
  textColor: "#111827",
  linkColor: "#0066cc",
  buttonColor: "#0066cc",
  buttonTextColor: "#ffffff",
  fontFamily: "Inter, sans-serif",
  containerWidth: 600,
}

const rendered: RenderPreviewResponse = {
  bodyHtml:
    "<!doctype html><html><body><p style=\"font-family:sans-serif\">Hello from the preview.</p></body></html>",
  bodyText: "Hello from the preview.",
  warnings: [],
}

// PreviewIframe debounces `doc` into `debouncedDoc` (initialized to `doc`) and
// reads `useQuery({ queryKey: ["render-preview", slug, debouncedDoc, theme,
// sample], ... })`. Seeding that exact key resolves the query synchronously
// with no BFF call, so the iframe srcdoc populates immediately.
function withSeededQuery(initialDoc: VisualDoc, initialTheme: Theme | null) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(
    ["render-preview", SLUG, initialDoc, initialTheme, null],
    rendered,
  )
  return client
}

const meta = {
  component: PreviewIframe,
  tags: ["ai-generated"],
  args: { slug: SLUG, doc, theme, sample: null },
  render: (args) => (
    <QueryClientProvider client={withSeededQuery(args.doc, args.theme)}>
      <PreviewIframe {...args} />
    </QueryClientProvider>
  ),
} satisfies Meta<typeof PreviewIframe>

export default meta
type Story = StoryObj<typeof meta>

export const Desktop: Story = {}

export const NoTheme: Story = {
  args: { theme: null },
}

// The viewport tablist swaps the active tab (and the iframe width) on click.
export const SwitchToMobile: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    const desktop = canvas.getByTestId("ve-preview-desktop")
    const mobile = canvas.getByTestId("ve-preview-mobile")
    await expect(desktop).toHaveAttribute("aria-selected", "true")
    await userEvent.click(mobile)
    await expect(mobile).toHaveAttribute("aria-selected", "true")
    await expect(desktop).toHaveAttribute("aria-selected", "false")
  },
}

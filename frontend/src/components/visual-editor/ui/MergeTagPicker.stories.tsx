import { useState } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, userEvent, within } from "storybook/test"
import { MergeTagPicker } from "./MergeTagPicker"
import type { MergeTagsResponse } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

const SLUG = "acme"

const data: MergeTagsResponse = {
  subscriber: [
    { slug: "first_name", displayName: "First name", type: "text", builtIn: true },
    { slug: "email", displayName: "Email", type: "text", builtIn: true },
  ],
  campaign: [{ key: "subject", displayName: "Subject" }],
}

// Seed the cache so useQuery resolves synchronously with no network — the
// picker reads queryKeys.mergeTags(SLUG).
function withSeededQuery(Story: () => React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(queryKeys.mergeTags(SLUG), data)
  return <QueryClientProvider client={client}>{Story()}</QueryClientProvider>
}

// The picker is controlled-open here so it renders without an editor instance.
function Harness() {
  const [open, setOpen] = useState(true)
  return (
    <MergeTagPicker slug={SLUG} editor={null} open={open} onOpenChange={setOpen} />
  )
}

const meta = {
  component: MergeTagPicker,
  decorators: [withSeededQuery],
  // slug + editor are required props; supply them at the meta level so each
  // story can render through its stateful Harness. The render functions
  // ignore these.
  args: { slug: SLUG, editor: null },
} satisfies Meta<typeof MergeTagPicker>

export default meta
type Story = StoryObj<typeof meta>

export const Open: Story = {
  render: () => <Harness />,
}

// Typing in the filter narrows the subscriber list.
export const Filtering: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      canvas.getByTestId("ve-merge-tag-item-subscriber-first_name"),
    ).toBeInTheDocument()
    await userEvent.type(canvas.getByTestId("ve-merge-tag-filter"), "email")
    await expect(
      canvas.getByTestId("ve-merge-tag-item-subscriber-email"),
    ).toBeInTheDocument()
    await expect(
      canvas.queryByTestId("ve-merge-tag-item-subscriber-first_name"),
    ).not.toBeInTheDocument()
  },
}

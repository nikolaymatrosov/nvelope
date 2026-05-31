import { useState } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, fireEvent, fn, waitFor, within } from "storybook/test"
import { MediaPicker } from "./media-picker"
import type { MediaAssetView } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

const SLUG = "acme"

function asset(overrides: Partial<MediaAssetView> = {}): MediaAssetView {
  return {
    id: "a1",
    filename: "logo.png",
    content_type: "image/png",
    size_bytes: 4096,
    public_url: "https://placehold.co/200x200/png",
    created_at: "2026-05-01T00:00:00Z",
    ...overrides,
  }
}

// Seed the cache so the picker's useQuery resolves synchronously with no
// network — the query key is queryKeys.media(SLUG) and the queryFn returns the
// `{ items }` payload. Each story passes the items it wants pre-loaded.
function seededDecorator(items: Array<MediaAssetView>) {
  return function Decorator(Story: () => React.ReactElement) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(queryKeys.media(SLUG), { items })
    return <QueryClientProvider client={client}>{Story()}</QueryClientProvider>
  }
}

// Controlled-open harness so the dialog renders without a parent trigger.
function Harness({ onPick }: { onPick?: (a: MediaAssetView) => void }) {
  const [open, setOpen] = useState(true)
  return (
    <MediaPicker
      slug={SLUG}
      open={open}
      onOpenChange={setOpen}
      onPick={(a) => onPick?.(a)}
    />
  )
}

const meta = {
  component: MediaPicker,
  tags: ["ai-generated"],
  args: { slug: SLUG, open: true, onOpenChange: fn(), onPick: fn() },
} satisfies Meta<typeof MediaPicker>

export default meta
type Story = StoryObj<typeof meta>

// A populated library renders the asset grid.
export const Populated: Story = {
  decorators: [
    seededDecorator([
      asset(),
      asset({ id: "a2", filename: "banner.jpg", content_type: "image/jpeg" }),
      asset({
        id: "a3",
        filename: "terms.pdf",
        content_type: "application/pdf",
      }),
    ]),
  ],
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    // Dialog content portals to document.body, not the story canvas.
    const body = within(canvasElement.ownerDocument.body)
    await expect(body.getByTestId("media-picker-grid")).toBeVisible()
    await expect(body.getByTestId("media-picker-item-a1")).toBeVisible()
    await expect(body.getByTestId("media-picker-item-a3")).toBeVisible()
  },
}

// No assets → the empty state with a link to the library; no grid.
export const EmptyLibrary: Story = {
  decorators: [seededDecorator([])],
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    await expect(body.queryByTestId("media-picker-grid")).not.toBeInTheDocument()
  },
}

// Clicking an asset fires onPick with that asset, then closes the dialog.
export const PicksAsset: Story = {
  decorators: [seededDecorator([asset()])],
  args: { onPick: fn() },
  render: (args) => <Harness onPick={args.onPick} />,
  play: async ({ args, canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    const item = await body.findByTestId("media-picker-item-a1")
    // Radix Dialog's pointer-event guard intercepts user-event's synthetic
    // pointer sequence in the test env; fireEvent dispatches the click directly
    // like the component's unit test does.
    fireEvent.click(item)
    await waitFor(() =>
      expect(args.onPick).toHaveBeenCalledWith(
        expect.objectContaining({ id: "a1" }),
      ),
    )
  },
}

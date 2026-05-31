import { useEffect, useRef } from "react"
import { expect, fn, userEvent, within } from "storybook/test"
import { useSlashCommandMenu } from "./SlashCommandMenu"
import type { Meta, StoryObj } from "@storybook/react-vite"

// `useSlashCommandMenu()` returns a `menu` React node plus a `ref` whose
// `.api` is the imperative MenuApi the TipTap suggestion plugin would
// normally drive. We drive it directly here — no live editor needed: push a
// few items, wire `setOnSelect`, and `open()` at a fixed rect. The menu
// renders a `role="listbox"` of `role="option"` buttons.
//
// The items carry an `apply(editor, range)` closure in production, but the
// rendered menu only ever calls the registered `onSelect(item)` on
// click/Enter — never `apply` — so a no-op `apply` is fine for the story.
const noopApply = () => {}

const items = [
  { key: "paragraph", label: "Paragraph", apply: noopApply },
  { key: "heading-1", label: "Heading 1", apply: noopApply },
  { key: "bullet-list", label: "Bulleted list", apply: noopApply },
  {
    key: "merge-tag",
    label: "Merge tag",
    description: "Insert a subscriber or campaign placeholder",
    apply: noopApply,
  },
]

type HarnessProps = { onSelect: (item: { key: string }) => void }

function Harness({ onSelect }: HarnessProps) {
  const { ref, menu } = useSlashCommandMenu()
  const anchorRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const api = ref.current?.api
    if (!api) return
    api.setItems(items)
    api.setOnSelect(onSelect)
    // Anchor near the harness box so the fixed-position menu lands on-screen.
    api.open(anchorRef.current?.getBoundingClientRect())
  }, [ref, onSelect])

  return (
    <div>
      <div
        ref={anchorRef}
        style={{ height: 24, width: 240, border: "1px dashed #cbd5e1" }}
      >
        Type / here…
      </div>
      {menu}
    </div>
  )
}

const meta = {
  component: Harness,
  tags: ["ai-generated"],
  args: { onSelect: fn() },
  render: (args) => <Harness {...args} />,
} satisfies Meta<typeof Harness>

export default meta
type Story = StoryObj<typeof meta>

export const Open: Story = {}

// Clicking an item fires the registered onSelect with that item.
export const SelectFiresCallback: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement)
    const item = await canvas.findByTestId("ve-slash-item-heading-1")
    await userEvent.click(item)
    await expect(args.onSelect).toHaveBeenCalledTimes(1)
    await expect(args.onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ key: "heading-1" }),
    )
  },
}

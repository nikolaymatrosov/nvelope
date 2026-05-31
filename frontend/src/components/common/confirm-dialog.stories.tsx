import { expect, fn, within } from "storybook/test"
import { ConfirmDialog } from "./confirm-dialog"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: ConfirmDialog,
  tags: ["ai-generated"],
  args: {
    open: true,
    onOpenChange: fn(),
    onConfirm: fn(),
    title: "Delete sending domain",
    description: "This permanently removes example.com and its DNS records.",
    confirmLabel: "Delete",
    cancelLabel: "Cancel",
  },
} satisfies Meta<typeof ConfirmDialog>

export default meta
type Story = StoryObj<typeof meta>

// The dialog renders through a portal into document.body, so queries go through
// the canvas element's owner document rather than the (empty) story canvas.
export const Open: Story = {
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    await expect(
      await body.findByText("Delete sending domain"),
    ).toBeVisible()
  },
}

// Clicking the confirm action fires onConfirm exactly once.
export const Confirms: Story = {
  play: async ({ args, canvasElement, userEvent }) => {
    const body = within(canvasElement.ownerDocument.body)
    const confirm = await body.findByRole("button", { name: "Delete" })
    await userEvent.click(confirm)
    await expect(args.onConfirm).toHaveBeenCalledTimes(1)
  },
}

// While busy, both actions are disabled so the operation cannot double-fire.
export const Busy: Story = {
  args: { busy: true },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    const working = await body.findByRole("button", { name: "Working…" })
    await expect(working).toBeDisabled()
  },
}

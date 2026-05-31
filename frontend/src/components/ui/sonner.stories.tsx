import { expect, within } from "storybook/test"
import { toast } from "sonner"
import { Toaster } from "./sonner"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Toaster,
  tags: ["ai-generated"],
} satisfies Meta<typeof Toaster>

export default meta
type Story = StoryObj<typeof meta>

// Clicking the button emits a toast that appears in the document body.
export const Triggered: Story = {
  render: () => (
    <>
      <Toaster />
      <Button onClick={() => toast("Campaign scheduled")}>Notify</Button>
    </>
  ),
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("button", { name: "Notify" }))
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("Campaign scheduled")).toBeInTheDocument()
  },
}

// A success toast carries its own variant styling and icon.
export const Success: Story = {
  render: () => (
    <>
      <Toaster />
      <Button onClick={() => toast.success("Email sent")}>Send</Button>
    </>
  ),
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("button", { name: "Send" }))
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("Email sent")).toBeInTheDocument()
  },
}

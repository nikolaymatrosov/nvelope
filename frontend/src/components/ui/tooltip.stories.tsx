import { expect, within } from "storybook/test"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "./tooltip"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Tooltip,
  tags: ["ai-generated"],
} satisfies Meta<typeof Tooltip>

export default meta
type Story = StoryObj<typeof meta>

// Default-open tooltip so the content is visible in the portal.
export const Open: Story = {
  render: () => (
    <TooltipProvider>
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <Button variant="outline">Hover me</Button>
        </TooltipTrigger>
        <TooltipContent>Send a test email</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    // The content is duplicated for accessibility; at least one is present.
    await expect(
      (await body.findAllByText("Send a test email")).length
    ).toBeGreaterThan(0)
  },
}

// Hovering the trigger reveals the tooltip (delayDuration defaults to 0).
export const OnHover: Story = {
  render: () => (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline">Hover me</Button>
        </TooltipTrigger>
        <TooltipContent>Duplicate campaign</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ),
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.hover(canvas.getByRole("button", { name: "Hover me" }))
    const body = within(canvasElement.ownerDocument.body)
    await expect(
      (await body.findAllByText("Duplicate campaign")).length
    ).toBeGreaterThan(0)
  },
}

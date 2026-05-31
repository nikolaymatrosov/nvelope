import { expect, within } from "storybook/test"
import { Spinner } from "./spinner"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Spinner,
  tags: ["ai-generated"],
} satisfies Meta<typeof Spinner>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

export const Large: Story = {
  args: { className: "size-8" },
}

// Inline within a button to signal a pending action.
export const InButton: Story = {
  render: () => (
    <Button disabled>
      <Spinner />
      Sending...
    </Button>
  ),
}

// Exposes an accessible status role for screen readers.
export const HasStatusRole: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByRole("status")).toHaveAccessibleName("Loading")
  },
}

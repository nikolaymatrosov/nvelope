import { expect } from "storybook/test"
import { Progress } from "./progress"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Progress,
  args: { className: "w-64" },
  tags: ["ai-generated"],
} satisfies Meta<typeof Progress>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: { value: 60 },
}

export const Empty: Story = {
  args: { value: 0 },
}

export const Complete: Story = {
  args: { value: 100 },
}

// The value drives the indicator's translateX offset (100 - value).
export const ReflectsValue: Story = {
  args: { value: 42 },
  play: async ({ canvasElement }) => {
    const indicator = canvasElement.querySelector<HTMLElement>(
      '[data-slot="progress-indicator"]'
    )
    await expect(indicator).not.toBeNull()
    await expect(indicator!.style.transform).toBe("translateX(-58%)")
  },
}

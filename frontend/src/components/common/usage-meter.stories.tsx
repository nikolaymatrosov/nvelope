import { expect } from "storybook/test"
import { UsageMeter } from "./usage-meter"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: UsageMeter,
  tags: ["ai-generated"],
  args: { used: 4200, included: 10000 },
} satisfies Meta<typeof UsageMeter>

export default meta
type Story = StoryObj<typeof meta>

// Comfortably within the allowance — neutral tone, 42% consumed.
export const WithinAllowance: Story = {}

// At/above 80% the meter switches to the near-limit warning tone.
export const NearLimit: Story = {
  args: { used: 8500, included: 10000 },
}

// At or over the included amount the meter reports exhaustion. The percent is
// clamped to 100 even though usage exceeds the allowance — assert that clamp
// rather than the raw ratio.
export const Exhausted: Story = {
  args: { used: 12000, included: 10000 },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("100%")).toBeVisible()
  },
}

import { expect } from "storybook/test"
import { MetricTile } from "./metric-tile"
import { RateValue } from "./rate-value"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: MetricTile,
  tags: ["ai-generated"],
  args: { label: "Sent", value: 12345 },
} satisfies Meta<typeof MetricTile>

export default meta
type Story = StoryObj<typeof meta>

// Label + a thousands-separated count, no rate line.
export const CountOnly: Story = {}

// The optional rate line appears below the count when supplied.
export const WithRate: Story = {
  args: { label: "Opened", value: 6789, rate: <RateValue value={0.55} /> },
}

// Proof the shared preview loaded the app's Tailwind CSS: the count uses the
// `text-2xl` utility, which resolves to a 24px font-size. A `toBeVisible`
// assertion alone would pass even with no stylesheet — a concrete computed
// value is the only real evidence the styles are applied.
export const CssCheck: Story = {
  play: async ({ canvas }) => {
    const count = canvas.getByText("12,345")
    await expect(getComputedStyle(count).fontSize).toBe("24px")
  },
}

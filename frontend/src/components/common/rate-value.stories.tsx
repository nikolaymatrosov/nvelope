import { RateValue } from "./rate-value"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: RateValue,
  tags: ["ai-generated"],
  args: { value: 0.125 },
} satisfies Meta<typeof RateValue>

export default meta
type Story = StoryObj<typeof meta>

// A fractional rate keeps one digit by default → "12.5%".
export const Fractional: Story = {}

// A clean rate drops the trailing ".0" → "12%", not "12.0%".
export const WholePercent: Story = { args: { value: 0.12 } }

// A zero rate renders "0%" with no special-casing.
export const Zero: Story = { args: { value: 0 } }

// fractionDigits controls precision for non-integer percentages.
export const TwoDigits: Story = { args: { value: 0.12345, fractionDigits: 2 } }

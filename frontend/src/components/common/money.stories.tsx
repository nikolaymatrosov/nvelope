import { Money } from "./money"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Money,
  tags: ["ai-generated"],
  args: { amountMinor: 123456, currency: "RUB" },
} satisfies Meta<typeof Money>

export default meta
type Story = StoryObj<typeof meta>

// Default RUB rendering — minor units divided by 100 and localized.
export const Rubles: Story = {}

// A different ISO currency code switches the symbol.
export const Dollars: Story = {
  args: { amountMinor: 4999, currency: "USD" },
}

// A zero amount still renders with two fraction digits, not a bare "0".
export const Zero: Story = {
  args: { amountMinor: 0, currency: "USD" },
}

// An unknown / malformed currency code falls back to the manual format path
// ("<value> <code>") rather than throwing.
export const FallbackCode: Story = {
  args: { amountMinor: 250000, currency: "XX!" },
}

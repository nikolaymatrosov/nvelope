import { expect, within } from "storybook/test"
import { Input } from "./input"
import { Label } from "./label"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Input,
  args: { placeholder: "you@example.com" },
  tags: ["ai-generated"],
} satisfies Meta<typeof Input>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

export const Disabled: Story = {
  args: { disabled: true, value: "locked@example.com" },
}

// aria-invalid drives the destructive ring styling.
export const Invalid: Story = {
  args: { "aria-invalid": true, defaultValue: "not-an-email" },
}

export const WithLabel: Story = {
  render: (args) => (
    <div className="grid w-72 gap-1.5">
      <Label htmlFor="email">Email</Label>
      <Input {...args} id="email" type="email" />
    </div>
  ),
}

// Typed text reaches the underlying input value.
export const TypesText: Story = {
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    const input = canvas.getByPlaceholderText<HTMLInputElement>(
      "you@example.com"
    )
    await userEvent.type(input, "ada@nvelope.dev")
    await expect(input).toHaveValue("ada@nvelope.dev")
  },
}

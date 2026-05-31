import { expect, fn, within } from "storybook/test"
import { Checkbox } from "./checkbox"
import { Label } from "./label"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Checkbox,
  args: { onCheckedChange: fn() },
  tags: ["ai-generated"],
} satisfies Meta<typeof Checkbox>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

export const Checked: Story = {
  args: { defaultChecked: true },
}

export const Disabled: Story = {
  args: { disabled: true, defaultChecked: true },
}

export const WithLabel: Story = {
  render: (args) => (
    <Label>
      <Checkbox {...args} id="terms" />
      Accept terms and conditions
    </Label>
  ),
}

// Clicking toggles the checked state and fires onCheckedChange.
export const Toggles: Story = {
  play: async ({ args, canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    const checkbox = canvas.getByRole("checkbox")
    await expect(checkbox).toHaveAttribute("aria-checked", "false")
    await userEvent.click(checkbox)
    await expect(checkbox).toHaveAttribute("aria-checked", "true")
    await expect(args.onCheckedChange).toHaveBeenCalledWith(true)
  },
}

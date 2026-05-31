import { Label } from "./label"
import { Checkbox } from "./checkbox"
import { Input } from "./input"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Label,
  args: { children: "Email address" },
  tags: ["ai-generated"],
} satisfies Meta<typeof Label>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

// Associated with an input via htmlFor.
export const WithInput: Story = {
  render: () => (
    <div className="grid w-72 gap-1.5">
      <Label htmlFor="name">Full name</Label>
      <Input id="name" placeholder="Ada Lovelace" />
    </div>
  ),
}

// Wrapping a control places label text inline beside it.
export const WrappingCheckbox: Story = {
  render: () => (
    <Label>
      <Checkbox />
      Subscribe to the newsletter
    </Label>
  ),
}

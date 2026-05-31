import { expect, within } from "storybook/test"
import { Textarea } from "./textarea"
import { Label } from "./label"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Textarea,
  args: { placeholder: "Write your message..." },
  tags: ["ai-generated"],
} satisfies Meta<typeof Textarea>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

export const Disabled: Story = {
  args: { disabled: true, value: "This field is read only." },
}

// aria-invalid drives the destructive ring styling.
export const Invalid: Story = {
  args: { "aria-invalid": true, defaultValue: "Too short" },
}

export const WithLabel: Story = {
  render: (args) => (
    <div className="grid w-80 gap-1.5">
      <Label htmlFor="body">Email body</Label>
      <Textarea {...args} id="body" />
    </div>
  ),
}

// Typed text reaches the underlying textarea value.
export const TypesText: Story = {
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    const textarea = canvas.getByPlaceholderText<HTMLTextAreaElement>(
      "Write your message..."
    )
    await userEvent.type(textarea, "Hello subscribers!")
    await expect(textarea).toHaveValue("Hello subscribers!")
  },
}

import { expect, fn, within } from "storybook/test"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "./select"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Select,
  tags: ["ai-generated"],
} satisfies Meta<typeof Select>

export default meta
type Story = StoryObj<typeof meta>

// Static, closed trigger showing the placeholder.
export const Default: Story = {
  render: () => (
    <Select>
      <SelectTrigger className="w-48">
        <SelectValue placeholder="Pick a status" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>Status</SelectLabel>
          <SelectItem value="draft">Draft</SelectItem>
          <SelectItem value="scheduled">Scheduled</SelectItem>
          <SelectSeparator />
          <SelectItem value="sent">Sent</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  ),
}

// Opening the listbox and choosing an option updates the value and fires onValueChange.
export const Chooses: Story = {
  args: { onValueChange: fn() },
  render: (args) => (
    <Select {...args}>
      <SelectTrigger className="w-48">
        <SelectValue placeholder="Pick a status" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="draft">Draft</SelectItem>
        <SelectItem value="scheduled">Scheduled</SelectItem>
        <SelectItem value="sent">Sent</SelectItem>
      </SelectContent>
    </Select>
  ),
  play: async ({ args, canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("combobox"))
    const body = within(canvasElement.ownerDocument.body)
    const option = await body.findByRole("option", { name: "Scheduled" })
    await userEvent.click(option)
    await expect(args.onValueChange).toHaveBeenCalledWith("scheduled")
    await expect(canvas.getByText("Scheduled")).toBeVisible()
  },
}

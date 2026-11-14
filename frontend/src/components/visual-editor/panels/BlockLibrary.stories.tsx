import { expect, fn, userEvent, within } from "storybook/test"
import { BlockLibrary } from "./BlockLibrary"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: BlockLibrary,
  args: { open: true, onOpenChange: fn(), onInsert: fn() },
} satisfies Meta<typeof BlockLibrary>

export default meta
type Story = StoryObj<typeof meta>

export const Open: Story = {}

export const PickHeading: Story = {
  play: async ({ args }) => {
    // The sheet renders in a portal on document.body.
    const body = within(document.body)
    await userEvent.click(await body.findByTestId("ve-library-type-heading"))
    await userEvent.click(body.getByTestId("ve-library-item-heading-1"))
    await expect(args.onInsert).toHaveBeenCalled()
  },
}

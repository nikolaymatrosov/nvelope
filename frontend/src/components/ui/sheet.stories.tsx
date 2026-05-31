import { expect, fn, within } from "storybook/test"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "./sheet"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Sheet,
  tags: ["ai-generated"],
} satisfies Meta<typeof Sheet>

export default meta
type Story = StoryObj<typeof meta>

// Default-open so the panel content is visible in the portal.
export const Open: Story = {
  render: () => (
    <Sheet defaultOpen>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Filters</SheetTitle>
          <SheetDescription>Narrow down the campaign list.</SheetDescription>
        </SheetHeader>
        <SheetFooter>
          <SheetClose asChild>
            <Button variant="outline">Close</Button>
          </SheetClose>
          <Button>Apply</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  ),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("Filters")).toBeVisible()
  },
}

// Opening from the left side via the trigger.
export const FromTrigger: Story = {
  args: { onOpenChange: fn() },
  render: (args) => (
    <Sheet {...args}>
      <SheetTrigger asChild>
        <Button variant="outline">Open menu</Button>
      </SheetTrigger>
      <SheetContent side="left">
        <SheetHeader>
          <SheetTitle>Navigation</SheetTitle>
          <SheetDescription>Jump to a workspace section.</SheetDescription>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  ),
  play: async ({ args, canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("button", { name: "Open menu" }))
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("Navigation")).toBeInTheDocument()
    await expect(args.onOpenChange).toHaveBeenCalled()
  },
}

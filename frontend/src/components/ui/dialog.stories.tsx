import { expect, fn, within } from "storybook/test"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./dialog"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Dialog,
  tags: ["ai-generated"],
} satisfies Meta<typeof Dialog>

export default meta
type Story = StoryObj<typeof meta>

// Default-open so the portal content is visible without interaction.
export const Open: Story = {
  render: () => (
    <Dialog defaultOpen>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit segment</DialogTitle>
          <DialogDescription>
            Update the rules that define this audience.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button>Save</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  ),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("Edit segment")).toBeVisible()
  },
}

// Opening via the trigger renders the dialog into the portal.
export const FromTrigger: Story = {
  args: { onOpenChange: fn() },
  render: (args) => (
    <Dialog {...args}>
      <DialogTrigger asChild>
        <Button>Open dialog</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New template</DialogTitle>
          <DialogDescription>
            Give your template a memorable name.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button>Create</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  ),
  play: async ({ args, canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("button", { name: "Open dialog" }))
    const body = within(canvasElement.ownerDocument.body)
    await expect(await body.findByText("New template")).toBeInTheDocument()
    await expect(args.onOpenChange).toHaveBeenCalled()
  },
}

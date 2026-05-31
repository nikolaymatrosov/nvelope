import { Separator } from "./separator"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Separator,
  tags: ["ai-generated"],
} satisfies Meta<typeof Separator>

export default meta
type Story = StoryObj<typeof meta>

export const Horizontal: Story = {
  render: () => (
    <div className="w-64">
      <p className="text-sm">Profile</p>
      <Separator className="my-3" />
      <p className="text-sm text-muted-foreground">
        Manage your account settings.
      </p>
    </div>
  ),
}

export const Vertical: Story = {
  render: () => (
    <div className="flex h-6 items-center gap-3 text-sm">
      <span>Drafts</span>
      <Separator orientation="vertical" />
      <span>Sent</span>
      <Separator orientation="vertical" />
      <span>Archived</span>
    </div>
  ),
}

import { InboxIcon } from "lucide-react"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "./empty"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Empty,
  tags: ["ai-generated"],
} satisfies Meta<typeof Empty>

export default meta
type Story = StoryObj<typeof meta>

// The common shape: framed icon, title, description.
export const WithIcon: Story = {
  render: () => (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <InboxIcon />
        </EmptyMedia>
        <EmptyTitle>No campaigns yet</EmptyTitle>
        <EmptyDescription>
          Create your first campaign to start sending.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ),
}

// The same shell plus a call-to-action in the content slot.
export const WithAction: Story = {
  render: () => (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <InboxIcon />
        </EmptyMedia>
        <EmptyTitle>No segments</EmptyTitle>
        <EmptyDescription>
          Segments let you target a slice of your audience.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button size="sm">New segment</Button>
      </EmptyContent>
    </Empty>
  ),
}

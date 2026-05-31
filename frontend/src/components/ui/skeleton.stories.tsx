import { Skeleton } from "./skeleton"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Skeleton,
  tags: ["ai-generated"],
} satisfies Meta<typeof Skeleton>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => <Skeleton className="h-4 w-48" />,
}

// A typical loading placeholder for a list row: avatar plus two text lines.
export const CardPlaceholder: Story = {
  render: () => (
    <div className="flex w-72 items-center gap-3">
      <Skeleton className="size-10 rounded-full" />
      <div className="grid flex-1 gap-2">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    </div>
  ),
}

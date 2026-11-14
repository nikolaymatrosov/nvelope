import { VisualBodyPreview } from "./VisualBodyPreview"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: VisualBodyPreview,
} satisfies Meta<typeof VisualBodyPreview>

export default meta
type Story = StoryObj<typeof meta>

export const WithBody: Story = {
  args: { html: "<h1>Hello!</h1><p>Thanks for joining.</p>" },
}

export const Empty: Story = {
  args: { html: "" },
}

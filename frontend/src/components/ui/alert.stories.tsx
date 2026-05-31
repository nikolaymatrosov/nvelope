import { Alert, AlertDescription, AlertTitle } from "./alert"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Alert,
} satisfies Meta<typeof Alert>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: (args) => (
    <Alert {...args}>
      <AlertTitle>Heads up</AlertTitle>
      <AlertDescription>
        This is what an alert looks like in the sandbox.
      </AlertDescription>
    </Alert>
  ),
}

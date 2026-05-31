import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "./card"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Card,
  tags: ["ai-generated"],
} satisfies Meta<typeof Card>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => (
    <Card className="w-80">
      <CardHeader>
        <CardTitle>Welcome series</CardTitle>
        <CardDescription>Automated onboarding emails.</CardDescription>
      </CardHeader>
      <CardContent>Sent to every new subscriber over three days.</CardContent>
    </Card>
  ),
}

// Header action slot plus a footer with a call-to-action.
export const WithActionAndFooter: Story = {
  render: () => (
    <Card className="w-80">
      <CardHeader>
        <CardTitle>Monthly newsletter</CardTitle>
        <CardDescription>Draft saved 2 minutes ago.</CardDescription>
        <CardAction>
          <Button size="xs" variant="ghost">
            Edit
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>1,204 recipients in the target segment.</CardContent>
      <CardFooter>
        <Button size="sm">Schedule send</Button>
      </CardFooter>
    </Card>
  ),
}

// The compact density variant.
export const Small: Story = {
  render: () => (
    <Card size="sm" className="w-72">
      <CardHeader>
        <CardTitle>Quick stat</CardTitle>
        <CardDescription>Open rate last 7 days</CardDescription>
      </CardHeader>
      <CardContent className="text-2xl font-semibold">42%</CardContent>
    </Card>
  ),
}

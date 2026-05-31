import { expect } from "storybook/test"
import { AsyncState } from "./async-state"
import type { Meta, StoryObj } from "@storybook/react-vite"

// A fixed fixture type so the story args are concretely typed rather than
// `unknown`. The render-prop child receives the loaded data.
type Item = { id: string; name: string }

const loaded: Array<Item> = [
  { id: "1", name: "Welcome series" },
  { id: "2", name: "Monthly newsletter" },
]

function renderItems(items: Array<Item>) {
  return (
    <ul className="flex flex-col gap-1">
      {items.map((item) => (
        <li key={item.id}>{item.name}</li>
      ))}
    </ul>
  )
}

const meta = {
  component: AsyncState<Array<Item>>,
  tags: ["ai-generated", "autodoc"],
  args: {
    query: { isLoading: false, isError: false, error: null, data: loaded },
    isEmpty: (items) => items.length === 0,
    children: renderItems,
  },
} satisfies Meta<typeof AsyncState<Array<Item>>>

export default meta
type Story = StoryObj<typeof meta>

// Data present → the render-prop output inside the populated wrapper.
export const Populated: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByTestId("async-populated")).toBeVisible()
    await expect(canvas.getByText("Welcome series")).toBeVisible()
  },
}

// While loading the default skeleton stands in for the eventual content.
export const Loading: Story = {
  args: {
    query: { isLoading: true, isError: false, error: undefined, data: undefined },
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByTestId("async-loading")).toBeVisible()
  },
}

// An error surfaces the message and a retry affordance (refetch supplied).
export const ErrorWithRetry: Story = {
  args: {
    query: {
      isLoading: false,
      isError: true,
      error: new Error("Network unreachable"),
      data: undefined,
      refetch: () => {},
    },
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByTestId("async-error")).toBeVisible()
    await expect(canvas.getByText("Network unreachable")).toBeVisible()
  },
}

// Loaded but empty data → the empty state, not the populated wrapper.
export const EmptyData: Story = {
  args: {
    query: { isLoading: false, isError: false, error: null, data: [] },
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByTestId("async-empty")).toBeVisible()
  },
}

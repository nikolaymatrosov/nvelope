import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { BlockParamsPanel } from "./BlockParamsPanel"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness that mimics the shared selection model: it holds a fake
// selected node and re-applies attr patches the way the real editor does, so
// the panel's live-apply behavior is visible in Storybook.
function Harness({
  blockType,
  initialAttrs,
}: {
  blockType: string
  initialAttrs: Record<string, unknown>
}) {
  const [attrs, setAttrs] = useState<Record<string, unknown>>(initialAttrs)
  const selection: BlockSelection = {
    selectedPos: blockType ? 0 : null,
    selectedNode: blockType
      ? ({ type: { name: blockType }, attrs } as unknown as BlockSelection["selectedNode"])
      : null,
    selectBlock: () => {},
    updateSelectedAttrs: (patch) => setAttrs((prev) => ({ ...prev, ...patch })),
    clear: () => {},
  }
  return (
    <div style={{ width: 280 }}>
      <BlockParamsPanel selection={selection} />
    </div>
  )
}

const meta = {
  component: BlockParamsPanel,
  // The stories supply their own selection via render; these args only satisfy
  // the required-prop type (see VisualEmailEditor.stories).
  args: {
    selection: {
      selectedPos: null,
      selectedNode: null,
      selectBlock: () => {},
      updateSelectedAttrs: () => {},
      clear: () => {},
    },
  },
} satisfies Meta<typeof BlockParamsPanel>

export default meta
type Story = StoryObj<typeof meta>

export const Empty: Story = {
  render: () => {
    const selection: BlockSelection = {
      selectedPos: null,
      selectedNode: null,
      selectBlock: () => {},
      updateSelectedAttrs: () => {},
      clear: () => {},
    }
    return (
      <div style={{ width: 280 }}>
        <BlockParamsPanel selection={selection} />
      </div>
    )
  },
}

export const Button: Story = {
  render: () => (
    <Harness
      blockType="button"
      initialAttrs={{
        label: "Read more",
        href: "https://example.test/x",
        style: { backgroundColor: "#1a73e8", borderRadius: 8 },
      }}
    />
  ),
}

export const Paragraph: Story = {
  render: () => <Harness blockType="paragraph" initialAttrs={{}} />,
}

export const EditAndReset: Story = {
  render: () => (
    <Harness
      blockType="button"
      initialAttrs={{ label: "Go", href: "https://example.test/x" }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-button")).toBeInTheDocument()
    await userEvent.clear(canvas.getByTestId("ve-param-label"))
    await userEvent.type(canvas.getByTestId("ve-param-label"), "Buy now")
    await expect(canvas.getByTestId("ve-param-label")).toHaveValue("Buy now")
    await userEvent.click(canvas.getByTestId("ve-params-reset-all"))
    await expect(canvas.getByTestId("ve-params-button")).toBeInTheDocument()
  },
}

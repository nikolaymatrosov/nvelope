import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { DividerParams } from "./DividerParams"
import type { BlockStyle } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness mirroring how BlockParamsPanel feeds the form. The divider
// only exposes the rule line (border group) and the spacing above/below
// (padding group).
function Harness({ initialStyle = {} }: { initialStyle?: BlockStyle }) {
  const [style, setStyle] = useState<BlockStyle>(initialStyle)
  return (
    <div style={{ width: 280 }}>
      <DividerParams
        blockType="divider"
        attrs={{}}
        style={style}
        onStyleChange={(patch) => setStyle((prev) => ({ ...prev, ...patch }))}
        onAttrsChange={() => {}}
      />
    </div>
  )
}

// Meta-level args satisfy the required ParamFormProps for the story type; each
// story overrides via its own stateful `render`, so these defaults are unused.
const meta = {
  component: DividerParams,
  args: {
    blockType: "divider",
    attrs: {},
    style: {},
    onStyleChange: () => {},
    onAttrsChange: () => {},
  },
} satisfies Meta<typeof DividerParams>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => (
    <Harness initialStyle={{ borderWidth: 1, borderColor: "#cbd5e1", paddingTop: 16, paddingBottom: 16 }} />
  ),
}

export const EditsSpacing: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-divider")).toBeInTheDocument()

    // Spacing is a bounded numeric stepper (starts empty / inherited).
    await userEvent.type(canvas.getByTestId("ve-param-paddingTop"), "24")
    await expect(canvas.getByTestId("ve-param-paddingTop")).toHaveValue(24)
  },
}

import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { ColumnsParams } from "./ColumnsParams"
import type { BlockStyle } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness mirroring how BlockParamsPanel feeds the form. The columns
// container only exposes container-level styling (background, padding, corner
// radius, border); the column count is read-only because columns are
// added/removed structurally on the canvas, not here.
function Harness({
  blockType = "columns",
  initialAttrs = {},
  initialStyle = {},
}: {
  blockType?: string
  initialAttrs?: Record<string, unknown>
  initialStyle?: BlockStyle
}) {
  const [style, setStyle] = useState<BlockStyle>(initialStyle)
  return (
    <div style={{ width: 280 }}>
      <ColumnsParams
        blockType={blockType}
        attrs={initialAttrs}
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
  component: ColumnsParams,
  args: {
    blockType: "columns",
    attrs: {},
    style: {},
    onStyleChange: () => {},
    onAttrsChange: () => {},
  },
} satisfies Meta<typeof ColumnsParams>

export default meta
type Story = StoryObj<typeof meta>

// The container variant shows the read-only column count alongside the styling.
export const Columns: Story = {
  render: () => (
    <Harness
      blockType="columns"
      initialAttrs={{ count: 2 }}
      initialStyle={{ backgroundColor: "#f1f5f9", paddingTop: 16, borderRadius: 8 }}
    />
  ),
}

// A single column has no count, just the container styling.
export const SingleColumn: Story = {
  render: () => <Harness blockType="column" />,
}

export const EditsContainerStyle: Story = {
  render: () => <Harness blockType="columns" initialAttrs={{ count: 3 }} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-columns")).toBeInTheDocument()

    // Column count is read-only and reflects the structural attr.
    await expect(canvas.getByTestId("ve-param-column-count")).toBeInTheDocument()

    // Bounded numeric style field (corner radius, starts empty / inherited).
    await userEvent.type(canvas.getByTestId("ve-param-borderRadius"), "10")
    await expect(canvas.getByTestId("ve-param-borderRadius")).toHaveValue(10)
  },
}

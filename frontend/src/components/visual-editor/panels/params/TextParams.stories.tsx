import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { TextParams } from "./TextParams"
import type { BlockStyle } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness mirroring how BlockParamsPanel feeds the form. Heading
// additionally exposes a level select; other text blocks only show typography,
// alignment, and spacing.
function Harness({
  blockType = "paragraph",
  initialAttrs = {},
  initialStyle = {},
}: {
  blockType?: string
  initialAttrs?: Record<string, unknown>
  initialStyle?: BlockStyle
}) {
  const [attrs, setAttrs] = useState<Record<string, unknown>>(initialAttrs)
  const [style, setStyle] = useState<BlockStyle>(initialStyle)
  return (
    <div style={{ width: 280 }}>
      <TextParams
        blockType={blockType}
        attrs={attrs}
        style={style}
        onStyleChange={(patch) => setStyle((prev) => ({ ...prev, ...patch }))}
        onAttrsChange={(patch) => setAttrs((prev) => ({ ...prev, ...patch }))}
      />
    </div>
  )
}

// Meta-level args satisfy the required ParamFormProps for the story type; each
// story overrides via its own stateful `render`, so these defaults are unused.
const meta = {
  component: TextParams,
  args: {
    blockType: "paragraph",
    attrs: {},
    style: {},
    onStyleChange: () => {},
    onAttrsChange: () => {},
  },
} satisfies Meta<typeof TextParams>

export default meta
type Story = StoryObj<typeof meta>

export const Paragraph: Story = {
  render: () => <Harness initialStyle={{ color: "#1a73e8", fontSize: 16 }} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-text")).toBeInTheDocument()
    // Paragraphs have no heading-level control.
    await expect(canvas.queryByTestId("ve-param-level")).not.toBeInTheDocument()
  },
}

export const Heading: Story = {
  render: () => <Harness blockType="heading" initialAttrs={{ level: 1 }} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-text")).toBeInTheDocument()

    await userEvent.selectOptions(canvas.getByTestId("ve-param-level"), "2")
    await expect(canvas.getByTestId("ve-param-level")).toHaveValue("2")

    await userEvent.click(canvas.getByTestId("ve-param-align-center"))
    await expect(canvas.getByTestId("ve-param-align-center")).toHaveAttribute("aria-pressed", "true")
  },
}

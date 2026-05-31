import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { ImageParams } from "./ImageParams"
import type { BlockStyle } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness mirroring how BlockParamsPanel feeds the form. The image
// block exposes alt + link text inputs plus alignment, corner radius, and the
// border group (background and font don't apply to an image).
function Harness({
  initialAttrs = {},
  initialStyle = {},
}: {
  initialAttrs?: Record<string, unknown>
  initialStyle?: BlockStyle
}) {
  const [attrs, setAttrs] = useState<Record<string, unknown>>(initialAttrs)
  const [style, setStyle] = useState<BlockStyle>(initialStyle)
  return (
    <div style={{ width: 280 }}>
      <ImageParams
        blockType="image"
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
  component: ImageParams,
  args: {
    blockType: "image",
    attrs: {},
    style: {},
    onStyleChange: () => {},
    onAttrsChange: () => {},
  },
} satisfies Meta<typeof ImageParams>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => (
    <Harness
      initialAttrs={{ alt: "Product hero", href: "https://example.com" }}
      initialStyle={{ textAlign: "center", borderRadius: 8 }}
    />
  ),
}

export const EditsAltAndAlignment: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-image")).toBeInTheDocument()

    // Free-text alt attribute.
    await userEvent.type(canvas.getByTestId("ve-param-alt"), "Logo")
    await expect(canvas.getByTestId("ve-param-alt")).toHaveValue("Logo")

    // Alignment toggle.
    await userEvent.click(canvas.getByTestId("ve-param-align-center"))
    await expect(canvas.getByTestId("ve-param-align-center")).toHaveAttribute("aria-pressed", "true")
  },
}

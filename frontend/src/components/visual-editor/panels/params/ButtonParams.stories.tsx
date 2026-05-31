import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { ButtonParams } from "./ButtonParams"
import type { BlockStyle } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness mirroring how BlockParamsPanel feeds the form: each single
// patch is merged back into the live attrs/style so the controlled inputs
// reflect what was typed. The button exposes label + href text inputs plus the
// colour / font / radius / padding / border style fields.
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
      <ButtonParams
        blockType="button"
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
  component: ButtonParams,
  args: {
    blockType: "button",
    attrs: {},
    style: {},
    onStyleChange: () => {},
    onAttrsChange: () => {},
  },
} satisfies Meta<typeof ButtonParams>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => (
    <Harness
      initialAttrs={{ label: "Shop now", href: "https://example.com" }}
      initialStyle={{ backgroundColor: "#2563eb", color: "#ffffff", borderRadius: 6, paddingTop: 12 }}
    />
  ),
}

export const EditsLabelHrefAndStyle: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByTestId("ve-params-button")).toBeInTheDocument()

    // Free-text attrs.
    await userEvent.type(canvas.getByTestId("ve-param-label"), "Buy")
    await expect(canvas.getByTestId("ve-param-label")).toHaveValue("Buy")

    await userEvent.type(canvas.getByTestId("ve-param-href"), "https://acme.test")
    await expect(canvas.getByTestId("ve-param-href")).toHaveValue("https://acme.test")

    // Bounded numeric style field (corner radius, starts empty / inherited).
    await userEvent.type(canvas.getByTestId("ve-param-borderRadius"), "12")
    await expect(canvas.getByTestId("ve-param-borderRadius")).toHaveValue(12)
  },
}

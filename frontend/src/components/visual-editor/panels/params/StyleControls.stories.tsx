import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { StyleControls } from "./StyleControls"
import type { StyleField } from "./StyleControls"
import type { BlockStyle } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Every per-block form composes StyleControls and merges each single-key patch
// back into the style object, so this harness does the same to keep the
// controls controlled and reflecting their live values.
function Harness({
  fields,
  initialStyle = {},
}: {
  fields: ReadonlyArray<StyleField>
  initialStyle?: BlockStyle
}) {
  const [style, setStyle] = useState<BlockStyle>(initialStyle)
  return (
    <div style={{ width: 280 }}>
      <StyleControls
        value={style}
        fields={fields}
        onChange={(patch) => setStyle((prev) => ({ ...prev, ...patch }))}
      />
    </div>
  )
}

// The full field set exercises every control type: color, font select, numeric
// stepper, alignment toggle, and the padding + border groups.
const ALL_FIELDS: ReadonlyArray<StyleField> = [
  "backgroundColor",
  "color",
  "fontFamily",
  "fontWeight",
  "fontSize",
  "lineHeight",
  "textAlign",
  "padding",
  "borderRadius",
  "border",
]

// Meta-level args satisfy the required props for the story type; each story
// overrides via its own stateful `render`, so these defaults are unused.
const meta = {
  component: StyleControls,
  args: {
    value: {},
    fields: [],
    onChange: () => {},
  },
} satisfies Meta<typeof StyleControls>

export default meta
type Story = StoryObj<typeof meta>

export const AllControlsEmpty: Story = {
  render: () => <Harness fields={ALL_FIELDS} />,
}

export const AllControlsWithValues: Story = {
  render: () => (
    <Harness
      fields={ALL_FIELDS}
      initialStyle={{
        color: "#1a73e8",
        backgroundColor: "#f1f5f9",
        fontSize: 16,
        fontWeight: 700,
        lineHeight: 1.5,
        textAlign: "center",
        paddingTop: 16,
        borderRadius: 8,
        borderWidth: 1,
        borderStyle: "solid",
        borderColor: "#cbd5e1",
      }}
    />
  ),
}

export const EditsNumberAlignAndSelect: Story = {
  render: () => <Harness fields={ALL_FIELDS} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)

    // Numeric stepper (bounded, starts empty / inherited).
    await userEvent.type(canvas.getByTestId("ve-param-borderRadius"), "16")
    await expect(canvas.getByTestId("ve-param-borderRadius")).toHaveValue(16)

    // Alignment toggle.
    await userEvent.click(canvas.getByTestId("ve-param-align-center"))
    await expect(canvas.getByTestId("ve-param-align-center")).toHaveAttribute("aria-pressed", "true")

    // Curated select.
    await userEvent.selectOptions(canvas.getByTestId("ve-param-fontWeight"), "700")
    await expect(canvas.getByTestId("ve-param-fontWeight")).toHaveValue("700")
  },
}

// Regression: a bounded field whose minimum is > 1 (fontSize, min 8) must still
// accept a multi-digit value typed one keystroke at a time. Clamping each
// keystroke would corrupt "18" → the leading "1" clamps up to 8, then "8"
// appends to "88" and clamps down to the max 72. The field must show the value
// the user typed, while never letting the parent observe an out-of-range value.
export const TypesMultiDigitFontSize: Story = {
  render: () => <Harness fields={["fontSize"]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    const fontSize = canvas.getByTestId("ve-param-fontSize")

    await userEvent.type(fontSize, "18")
    await expect(fontSize).toHaveValue(18)
  },
}

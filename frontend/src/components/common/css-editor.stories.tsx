import { useState } from "react"
import { expect, fn } from "storybook/test"
import { CssEditor } from "./css-editor"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful wrapper so typing round-trips through value/onChange exactly as the
// settings form drives it. The supplied onChange spy still fires so plays can
// assert it was called.
function Harness({
  initial,
  limitBytes,
  sanitized,
  onChange,
}: {
  initial: string
  limitBytes: number
  sanitized?: string | null
  onChange?: (value: string) => void
}) {
  const [value, setValue] = useState(initial)
  return (
    <CssEditor
      value={value}
      onChange={(next) => {
        setValue(next)
        onChange?.(next)
      }}
      limitBytes={limitBytes}
      sanitized={sanitized}
    />
  )
}

const meta = {
  component: CssEditor,
  tags: ["ai-generated"],
  // All props are required; meta defaults let each story render through its
  // stateful Harness. The render functions ignore these.
  args: { value: "", onChange: fn(), limitBytes: 4096 },
} satisfies Meta<typeof CssEditor>

export default meta
type Story = StoryObj<typeof meta>

// Empty editor well under the size limit — counter shows 0 used, no error.
export const Empty: Story = {
  render: (args) => <Harness initial="" limitBytes={args.limitBytes} />,
}

// A small valid stylesheet within budget.
export const WithContent: Story = {
  render: (args) => (
    <Harness
      initial={".btn { color: #0066cc; font-weight: 600; }"}
      limitBytes={args.limitBytes}
    />
  ),
}

// Input exceeding the byte limit flips the counter destructive and surfaces an
// over-limit alert, and marks the textarea aria-invalid.
export const OverLimit: Story = {
  render: () => <Harness initial={"a".repeat(40)} limitBytes={16} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("alert")).toBeVisible()
    await expect(canvas.getByTestId("css-editor-input")).toHaveAttribute(
      "aria-invalid",
      "true",
    )
  },
}

// A server-sanitized copy renders as a read-only preview block beneath the
// input.
export const WithSanitizedPreview: Story = {
  render: () => (
    <Harness
      initial={".x { color: red; behavior: url(evil); }"}
      limitBytes={4096}
      sanitized={".x { color: red; }"}
    />
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByTestId("css-editor-sanitized")).toBeVisible()
  },
}

// Typing into the textarea drives onChange and updates the live byte counter.
export const Typing: Story = {
  render: (args) => (
    <Harness initial="" limitBytes={args.limitBytes} onChange={args.onChange} />
  ),
  play: async ({ canvas, args, userEvent }) => {
    const input = canvas.getByTestId("css-editor-input")
    await userEvent.type(input, "color: red")
    await expect(input).toHaveValue("color: red")
    await expect(args.onChange).toHaveBeenCalled()
  },
}

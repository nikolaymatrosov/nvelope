import { useState } from "react"
import { expect, fn, userEvent } from "storybook/test"
import { JsonAttributeEditor } from "./json-attribute-editor"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness mirroring the parent form: it holds the last-emitted object
// and forwards the spies so plays can assert on onChange / onValidityChange.
function Harness({
  initial,
  onChange,
  onValidityChange,
}: {
  initial: Record<string, unknown>
  onChange?: (next: Record<string, unknown>) => void
  onValidityChange?: (valid: boolean) => void
}) {
  const [value, setValue] = useState(initial)
  return (
    <JsonAttributeEditor
      value={value}
      onChange={(next) => {
        setValue(next)
        onChange?.(next)
      }}
      onValidityChange={onValidityChange}
    />
  )
}

const meta = {
  component: JsonAttributeEditor,
  tags: ["ai-generated"],
  args: { value: {}, onChange: fn(), onValidityChange: fn() },
} satisfies Meta<typeof JsonAttributeEditor>

export default meta
type Story = StoryObj<typeof meta>

// Empty object → seeded as "{}" with the hint line beneath.
export const Empty: Story = {
  render: (args) => (
    <Harness
      initial={{}}
      onChange={args.onChange}
      onValidityChange={args.onValidityChange}
    />
  ),
}

// A populated attribute object pretty-printed into the textarea.
export const Populated: Story = {
  render: (args) => (
    <Harness
      initial={{ plan: "pro", seats: 12, beta: true }}
      onChange={args.onChange}
      onValidityChange={args.onValidityChange}
    />
  ),
  play: async ({ canvas }) => {
    const input = canvas.getByLabelText(/custom attributes/i)
    await expect(input).toHaveValue(
      JSON.stringify({ plan: "pro", seats: 12, beta: true }, null, 2),
    )
  },
}

// Typing malformed JSON surfaces an alert and reports invalidity to the parent.
export const InvalidJson: Story = {
  render: (args) => (
    <Harness
      initial={{}}
      onChange={args.onChange}
      onValidityChange={args.onValidityChange}
    />
  ),
  play: async ({ canvas, args }) => {
    const input = canvas.getByLabelText(/custom attributes/i)
    await userEvent.clear(input)
    // `{{` escapes to a literal `{` in user-event's keyboard syntax.
    await userEvent.type(input, "{{ not json")
    await expect(canvas.getByRole("alert")).toBeVisible()
    await expect(args.onValidityChange).toHaveBeenLastCalledWith(false)
  },
}

// Entering a JSON array (not an object) is rejected with a type error.
export const NonObjectRejected: Story = {
  render: (args) => (
    <Harness
      initial={{}}
      onChange={args.onChange}
      onValidityChange={args.onValidityChange}
    />
  ),
  play: async ({ canvas, args }) => {
    const input = canvas.getByLabelText(/custom attributes/i)
    await userEvent.clear(input)
    // `[[` escapes to a literal `[` in user-event's keyboard syntax.
    await userEvent.type(input, "[[1, 2, 3]")
    await expect(canvas.getByText(/must be a json object/i)).toBeVisible()
    await expect(args.onValidityChange).toHaveBeenLastCalledWith(false)
  },
}

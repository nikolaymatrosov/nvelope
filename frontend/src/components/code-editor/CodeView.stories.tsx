import { useState } from "react"
import { expect, fn, userEvent, within } from "storybook/test"
import { CodeView } from "./CodeView"
import type { Meta, StoryObj } from "@storybook/react-vite"

const SAMPLE_HTML = `<!doctype html>
<html>
  <body>
    <h1>Hello {{ first_name }}</h1>
    <p>Welcome to the newsletter.</p>
  </body>
</html>
`

const meta = {
  component: CodeView,
  tags: ["ai-generated"],
  args: { value: SAMPLE_HTML, onChange: fn() },
} satisfies Meta<typeof CodeView>

export default meta
type Story = StoryObj<typeof meta>

// The default editable editor with HTML highlighting and line numbers.
export const Editable: Story = {
  args: { className: "h-72 w-[40rem] border rounded-md overflow-hidden" },
}

// A read-only preview of server-rendered HTML.
export const ReadOnly: Story = {
  args: {
    editable: false,
    className: "h-72 w-[40rem] border rounded-md overflow-hidden",
  },
}

// An empty editor shows the placeholder.
export const WithPlaceholder: Story = {
  args: {
    value: "",
    placeholder: "Paste your HTML here…",
    className: "h-72 w-[40rem] border rounded-md overflow-hidden",
  },
}

// Stateful harness so typing round-trips through onChange like the real hosts.
function TypingHarness({ onChange }: { onChange: (next: string) => void }) {
  const [value, setValue] = useState("")
  return (
    <CodeView
      value={value}
      onChange={(next) => {
        setValue(next)
        onChange(next)
      }}
      placeholder="Type here…"
      ariaLabel="HTML editor"
      className="h-72 w-[40rem] border rounded-md overflow-hidden"
    />
  )
}

// Typing into the editor fires onChange with the typed text.
export const TypingFiresOnChange: Story = {
  render: (args) => <TypingHarness onChange={args.onChange} />,
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement)
    // CodeMirror renders a contenteditable surface; the textbox role targets it.
    const editor = canvas.getByRole("textbox")
    await userEvent.click(editor)
    await userEvent.type(editor, "<p>hi</p>")
    await expect(args.onChange).toHaveBeenCalled()
  },
}

import { useEffect } from "react"
import { Color } from "@tiptap/extension-color"
import { TextStyle } from "@tiptap/extension-text-style"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { expect, userEvent, within } from "storybook/test"
import { VisualBubbleMenu } from "./BubbleMenu"
import type { Meta, StoryObj } from "@storybook/react-vite"

// VisualBubbleMenu wraps `@tiptap/react/menus`' BubbleMenu, which only mounts
// its DOM once the editor reports a non-empty text selection. We build a real
// editor with the marks the menu toggles (bold/italic/strike, link, color)
// plus headings, seed it with a paragraph, then select all text so the menu
// becomes visible. The bubble DOM is portaled into the editor host, so play
// functions must query the whole document body.
function Harness() {
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] },
        link: { openOnClick: false, autolink: false },
      }),
      TextStyle,
      Color,
    ],
    content: "<p>Select this text to reveal the bubble menu.</p>",
  })

  useEffect(() => {
    editor.commands.focus()
    editor.commands.selectAll()
  }, [editor])

  return (
    <div style={{ minWidth: 360, padding: 40 }}>
      <EditorContent editor={editor} />
      <VisualBubbleMenu editor={editor} />
    </div>
  )
}

const meta = {
  component: VisualBubbleMenu,
  tags: ["ai-generated"],
  args: { editor: null },
  render: () => <Harness />,
} satisfies Meta<typeof VisualBubbleMenu>

export default meta
type Story = StoryObj<typeof meta>

export const OnSelection: Story = {}

// With all text selected the menu shows; clicking Bold toggles the bold mark.
export const ToggleBold: Story = {
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body)
    const boldBtn = await body.findByTestId(
      "ve-bm-bold",
      {},
      { timeout: 3000 },
    )
    // The button toggles the bold mark across the selection; assert the editor
    // produced a <strong> in its serialized HTML (the button itself only
    // signals active state via inline background, not an ARIA attribute).
    await userEvent.click(boldBtn)
    const editorEl = canvasElement.querySelector(".ProseMirror")
    await expect(editorEl?.querySelector("strong")).not.toBeNull()
  },
}

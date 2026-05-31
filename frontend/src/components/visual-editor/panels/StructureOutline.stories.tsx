import { useEffect, useState } from "react"
import { expect, within } from "storybook/test"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Column, Columns } from "../extensions/Columns"
import { Divider } from "../extensions/Divider"
import { useBlockSelection } from "../hooks/useBlockSelection"
import { StructureOutline } from "./StructureOutline"
import type { Editor } from "@tiptap/core"
import type { Meta, StoryObj } from "@storybook/react-vite"

const sampleDoc = {
  type: "doc",
  content: [
    { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "Newsletter" }] },
    { type: "paragraph", content: [{ type: "text", text: "Welcome to the spring edition." }] },
    {
      type: "columns",
      attrs: { count: 2 },
      content: [
        { type: "column", content: [{ type: "paragraph", content: [{ type: "text", text: "Left column" }] }] },
        { type: "column", content: [{ type: "paragraph", content: [{ type: "text", text: "Right column" }] }] },
      ],
    },
    { type: "divider" },
  ],
}

// Mounts a real (headless) editor so the outline projects a genuine document.
function Harness() {
  const editor = useEditor({
    extensions: [StarterKit.configure({ hardBreak: false }), Columns, Column, Divider],
    content: sampleDoc,
  })
  const selection = useBlockSelection(editor)
  return (
    <div style={{ display: "flex", gap: 16 }}>
      <div style={{ width: 260, border: "1px solid #e5e7eb" }}>
        <StructureOutline editor={editor} selection={selection} />
      </div>
      <div style={{ flex: 1 }}>
        <EditorContent editor={editor} />
      </div>
    </div>
  )
}

const meta = {
  component: StructureOutline,
} satisfies Meta<typeof StructureOutline>

export default meta
type Story = StoryObj<typeof meta>

export const NestedDocument: Story = {
  render: () => <Harness />,
}

export const ShowsTheHierarchy: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(await canvas.findByTestId("ve-outline")).toBeInTheDocument()
    await expect(canvas.getByTestId("ve-outline-row-heading")).toBeInTheDocument()
    await expect(canvas.getByTestId("ve-outline-row-columns")).toBeInTheDocument()
  },
}

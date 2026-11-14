import { fn } from "storybook/test"
import { EditorTopBar } from "./EditorTopBar"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: EditorTopBar,
  args: {
    section: "Templates",
    name: "Welcome email",
    onBack: fn(),
    editable: true,
    device: "desktop",
    onDeviceChange: fn(),
    canUndo: true,
    canRedo: false,
    onUndo: fn(),
    onRedo: fn(),
    onAdd: fn(),
    onPreview: fn(),
    dirty: true,
    saving: false,
    onSave: fn(),
  },
} satisfies Meta<typeof EditorTopBar>

export default meta
type Story = StoryObj<typeof meta>

export const Editing: Story = {}
export const Saved: Story = { args: { dirty: false } }
export const ReadOnly: Story = { args: { editable: false } }

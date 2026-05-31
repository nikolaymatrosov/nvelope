import { useState } from "react"
import { expect, fn, userEvent, within } from "storybook/test"
import { ThemeControls } from "./ThemeControls"
import type { Theme } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

const resolved: Theme = {
  textColor: "#111827",
  linkColor: "#0066cc",
  buttonColor: "#0066cc",
  buttonTextColor: "#ffffff",
  fontFamily: "Inter, sans-serif",
  containerWidth: 600,
}

// Stateful wrapper so the override round-trips through onChange like it does
// in the real editor chrome.
function Harness({ initial }: { initial: Theme | null }) {
  const [value, setValue] = useState<Theme | null>(initial)
  return <ThemeControls value={value} resolved={resolved} onChange={setValue} />
}

const meta = {
  component: ThemeControls,
  // The component's props are all required; supply defaults at the meta level
  // so each story can drive rendering through its stateful Harness without
  // repeating args. The render functions ignore these.
  args: { value: null, resolved, onChange: fn() },
} satisfies Meta<typeof ThemeControls>

export default meta
type Story = StoryObj<typeof meta>

// Inheriting tenant branding (value === null).
export const Inheriting: Story = {
  render: () => <Harness initial={null} />,
}

// A pinned override with editable fields.
export const Pinned: Story = {
  render: () => <Harness initial={resolved} />,
}

// Pin → the editable body appears; reset → back to the inherit badge.
export const PinThenReset: Story = {
  render: () => <Harness initial={null} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      canvas.getByTestId("ve-theme-inherit-badge"),
    ).toBeInTheDocument()
    await userEvent.click(canvas.getByTestId("ve-theme-pin-override"))
    await expect(
      canvas.getByTestId("ve-theme-pinned-body"),
    ).toBeInTheDocument()
    await userEvent.click(canvas.getByTestId("ve-theme-reset-defaults"))
    await expect(
      canvas.getByTestId("ve-theme-inherit-badge"),
    ).toBeInTheDocument()
  },
}

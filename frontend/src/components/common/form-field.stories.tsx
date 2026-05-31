import { expect } from "storybook/test"
import { FormField } from "./form-field"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: FormField,
  tags: ["ai-generated", "autodoc"],
  args: { label: "Workspace name", placeholder: "Acme Inc." },
} satisfies Meta<typeof FormField>

export default meta
type Story = StoryObj<typeof meta>

// Plain field with a hint line below the input.
export const WithHint: Story = {
  args: { hint: "Shown on invoices and the public archive." },
}

// A required field renders an asterisk after the label.
export const Required: Story = {
  args: { required: true },
  play: async ({ canvas }) => {
    // The "*" lives in its own span next to the label text.
    await expect(canvas.getByText("*")).toBeVisible()
  },
}

// An error replaces the hint and exposes an alert role; the input is marked
// aria-invalid so assistive tech announces the failure.
export const WithError: Story = {
  args: {
    hint: "Shown on invoices.",
    error: "Workspace name is required.",
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("alert")).toHaveTextContent(
      "Workspace name is required.",
    )
    await expect(canvas.getByRole("textbox")).toHaveAttribute(
      "aria-invalid",
      "true",
    )
  },
}

// Typing into the field is reflected in its value (smoke interaction).
export const Typing: Story = {
  play: async ({ canvas, userEvent }) => {
    const input = canvas.getByRole("textbox")
    await userEvent.type(input, "Acme")
    await expect(input).toHaveValue("Acme")
  },
}

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, fn, userEvent, within } from "storybook/test"
import { TotpChallenge } from "./totp-challenge"
import type { Meta, StoryObj } from "@storybook/react-vite"

// TotpChallenge uses useMutation (verifySessionTOTP) but issues the request only
// on submit. A local QueryClientProvider satisfies the hook; stories that do not
// submit never hit the network.
function withClient(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

const meta = {
  component: TotpChallenge,
  tags: ["ai-generated"],
  args: { slug: "acme", onVerified: fn() },
  render: (args) => withClient(<TotpChallenge {...args} />),
} satisfies Meta<typeof TotpChallenge>

export default meta
type Story = StoryObj<typeof meta>

// The verification card with the code field and submit button.
export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      canvas.getByText("Two-factor verification"),
    ).toBeInTheDocument()
    await expect(
      canvas.getByRole("button", { name: "Verify" }),
    ).toBeInTheDocument()
  },
}

// Submitting with an empty code surfaces the required-field validation message
// without ever issuing the network mutation.
export const RequiresCode: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await userEvent.click(canvas.getByRole("button", { name: "Verify" }))
    await expect(
      await canvas.findByText("Enter the 6-digit code."),
    ).toBeInTheDocument()
  },
}

// The code can be typed into the field.
export const TypingCode: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    const field = canvas.getByLabelText("Authentication code")
    await userEvent.type(field, "123456")
    await expect(field).toHaveValue("123456")
  },
}

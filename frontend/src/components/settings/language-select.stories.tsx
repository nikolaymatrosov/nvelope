import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, within } from "storybook/test"
import { LanguageSelect } from "./language-select"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

// useLocale → useSession reads the `me()` query. Seed it so the component
// renders without a network round-trip; a signed-out account keeps the choice
// cookie-only (no updateMyLocale call), which is the simplest harness.
function withClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  client.setQueryData(queryKeys.me(), undefined)
  return client
}

const meta = {
  component: LanguageSelect,
  tags: ["ai-generated"],
  render: () => (
    <QueryClientProvider client={withClient()}>
      <LanguageSelect />
    </QueryClientProvider>
  ),
} satisfies Meta<typeof LanguageSelect>

export default meta
type Story = StoryObj<typeof meta>

// The default switcher renders its label and a trigger showing the active locale.
export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByRole("combobox")).toBeInTheDocument()
  },
}

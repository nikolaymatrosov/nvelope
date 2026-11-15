import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { VisualBodyPreview } from "./VisualBodyPreview"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { queryKeys } from "@/lib/query"

const meta = {
  component: VisualBodyPreview,
  decorators: [
    (Story) => {
      // usePreviewHtml reads the media listing; seed it so no request fires.
      const client = new QueryClient()
      client.setQueryData(queryKeys.media("acme"), { items: [] })
      return (
        <QueryClientProvider client={client}>
          <Story />
        </QueryClientProvider>
      )
    },
  ],
} satisfies Meta<typeof VisualBodyPreview>

export default meta
type Story = StoryObj<typeof meta>

export const WithBody: Story = {
  args: { slug: "acme", html: "<h1>Hello!</h1><p>Thanks for joining.</p>" },
}

export const Empty: Story = {
  args: { slug: "acme", html: "" },
}

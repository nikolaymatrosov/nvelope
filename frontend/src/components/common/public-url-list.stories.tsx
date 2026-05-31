import { expect } from "storybook/test"
import { PublicUrlList } from "./public-url-list"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: PublicUrlList,
  tags: ["ai-generated"],
  args: {
    rows: [
      {
        label: "Subscription page",
        url: "https://acme.nvelope.app/s/subscribe",
        kind: "subscription",
      },
      {
        label: "Preference link template",
        url: "https://acme.nvelope.app/s/prefs/{token}",
        previewUrl: "https://acme.nvelope.app/s/prefs/example",
        kind: "preference-template",
      },
      {
        label: "Public archive",
        url: "https://acme.nvelope.app/archive",
        kind: "archive",
      },
    ],
  },
} satisfies Meta<typeof PublicUrlList>

export default meta
type Story = StoryObj<typeof meta>

// The populated list with the three public-URL kinds.
export const Populated: Story = {}

// The preference-template row carries an explanatory token hint and, unlike the
// other kinds, no preview button (its URL contains a per-subscriber token).
export const TokenTemplateRow: Story = {
  play: async ({ canvas }) => {
    const row = canvas.getByTestId("public-url-row-preference-template")
    await expect(
      canvas.queryByLabelText(/preview Preference link template/i),
    ).toBeNull()
    await expect(row).toBeVisible()
  },
}

// No rows → an explicit empty message instead of a blank list.
export const Empty: Story = {
  args: { rows: [] },
  play: async ({ canvas }) => {
    await expect(canvas.getByTestId("public-url-list-empty")).toBeVisible()
  },
}

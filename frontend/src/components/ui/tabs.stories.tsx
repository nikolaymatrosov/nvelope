import { expect, within } from "storybook/test"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./tabs"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Tabs,
  tags: ["ai-generated"],
} satisfies Meta<typeof Tabs>

export default meta
type Story = StoryObj<typeof meta>

// Static default with the first panel active.
export const Default: Story = {
  render: () => (
    <Tabs defaultValue="overview" className="w-80">
      <TabsList>
        <TabsTrigger value="overview">Overview</TabsTrigger>
        <TabsTrigger value="recipients">Recipients</TabsTrigger>
        <TabsTrigger value="settings">Settings</TabsTrigger>
      </TabsList>
      <TabsContent value="overview">Campaign overview and stats.</TabsContent>
      <TabsContent value="recipients">1,204 recipients targeted.</TabsContent>
      <TabsContent value="settings">Sending domain and schedule.</TabsContent>
    </Tabs>
  ),
}

// The "line" list variant.
export const LineVariant: Story = {
  render: () => (
    <Tabs defaultValue="overview" className="w-80">
      <TabsList variant="line">
        <TabsTrigger value="overview">Overview</TabsTrigger>
        <TabsTrigger value="recipients">Recipients</TabsTrigger>
      </TabsList>
      <TabsContent value="overview">Campaign overview and stats.</TabsContent>
      <TabsContent value="recipients">1,204 recipients targeted.</TabsContent>
    </Tabs>
  ),
}

// Switching tabs reveals the matching panel.
export const SwitchesPanel: Story = {
  render: () => (
    <Tabs defaultValue="overview" className="w-80">
      <TabsList>
        <TabsTrigger value="overview">Overview</TabsTrigger>
        <TabsTrigger value="recipients">Recipients</TabsTrigger>
      </TabsList>
      <TabsContent value="overview">Campaign overview and stats.</TabsContent>
      <TabsContent value="recipients">1,204 recipients targeted.</TabsContent>
    </Tabs>
  ),
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement)
    await expect(canvas.getByText("Campaign overview and stats.")).toBeVisible()
    await userEvent.click(canvas.getByRole("tab", { name: "Recipients" }))
    await expect(canvas.getByText("1,204 recipients targeted.")).toBeVisible()
  },
}

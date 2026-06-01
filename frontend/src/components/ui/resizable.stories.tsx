import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "./resizable"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: ResizablePanelGroup,
  tags: ["ai-generated"],
} satisfies Meta<typeof ResizablePanelGroup>

export default meta
type Story = StoryObj<typeof meta>

const Cell = ({ label }: { label: string }) => (
  <div className="flex h-full items-center justify-center p-4 text-sm text-muted-foreground">
    {label}
  </div>
)

export const Horizontal: Story = {
  render: () => (
    <div className="h-48 w-96 rounded-lg border">
      <ResizablePanelGroup orientation="horizontal">
        <ResizablePanel defaultSize={40}>
          <Cell label="Left" />
        </ResizablePanel>
        <ResizableHandle />
        <ResizablePanel defaultSize={60}>
          <Cell label="Right" />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  ),
}

export const Vertical: Story = {
  render: () => (
    <div className="h-64 w-96 rounded-lg border">
      <ResizablePanelGroup orientation="vertical">
        <ResizablePanel defaultSize={50}>
          <Cell label="Top" />
        </ResizablePanel>
        <ResizableHandle />
        <ResizablePanel defaultSize={50}>
          <Cell label="Bottom" />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  ),
}

// withHandle renders the visible drag grip in the middle of the separator.
export const WithHandle: Story = {
  render: () => (
    <div className="h-48 w-96 rounded-lg border">
      <ResizablePanelGroup orientation="horizontal">
        <ResizablePanel defaultSize={33}>
          <Cell label="Sidebar" />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize={67}>
          <Cell label="Content" />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  ),
}

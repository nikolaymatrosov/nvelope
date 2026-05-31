import { useState } from "react"
import { expect, fn, userEvent } from "storybook/test"
import { DataTable } from "./data-table"
import type { ColumnDef } from "./data-table"
import type { Meta, StoryObj } from "@storybook/react-vite"

// A concrete row type so columns and args are typed rather than `unknown`.
type Subscriber = { id: string; email: string; name: string; state: string }

const columns: Array<ColumnDef<Subscriber, unknown>> = [
  { accessorKey: "email", header: "Email" },
  { accessorKey: "name", header: "Name" },
  { accessorKey: "state", header: "State" },
]

// 23 rows so manual server paging is exercised across multiple pages.
const all: Array<Subscriber> = Array.from({ length: 23 }, (_, i) => ({
  id: `s${i + 1}`,
  email: `user${i + 1}@example.test`,
  name: `User ${i + 1}`,
  state: i % 2 === 0 ? "subscribed" : "unsubscribed",
}))

// Stateful harness emulating the server: it slices `all` by offset/limit and
// re-pages on demand, mirroring a real `{ items, total }` endpoint.
function Harness({
  limit = 10,
  onPageChange,
  onRowClick,
}: {
  limit?: number
  onPageChange?: (offset: number) => void
  onRowClick?: (row: Subscriber) => void
}) {
  const [offset, setOffset] = useState(0)
  const rows = all.slice(offset, offset + limit)
  return (
    <DataTable<Subscriber>
      columns={columns}
      rows={rows}
      total={all.length}
      limit={limit}
      offset={offset}
      onPageChange={(next) => {
        setOffset(next)
        onPageChange?.(next)
      }}
      getRowId={(row) => row.id}
      onRowClick={onRowClick}
    />
  )
}

const meta = {
  component: DataTable<Subscriber>,
  tags: ["ai-generated"],
  // Required props supplied at meta level; stories render through the harness.
  args: {
    columns,
    rows: all.slice(0, 10),
    total: all.length,
    limit: 10,
    offset: 0,
    onPageChange: fn(),
    getRowId: (row: Subscriber) => row.id,
  },
} satisfies Meta<typeof DataTable<Subscriber>>

export default meta
type Story = StoryObj<typeof meta>

// First page of a multi-page result. Previous is disabled; Next is enabled.
export const FirstPage: Story = {
  render: () => <Harness />,
  play: async ({ canvas }) => {
    await expect(canvas.getByText("user1@example.test")).toBeVisible()
    await expect(canvas.getByText("1–10 of 23")).toBeVisible()
    await expect(
      canvas.getByRole("button", { name: /previous/i }),
    ).toBeDisabled()
    await expect(canvas.getByRole("button", { name: /next/i })).toBeEnabled()
  },
}

// Empty result set — header still renders, range collapses to "0–0 of 0",
// both pager buttons disabled.
export const Empty: Story = {
  render: () => (
    <DataTable<Subscriber>
      columns={columns}
      rows={[]}
      total={0}
      limit={10}
      offset={0}
      onPageChange={fn()}
      getRowId={(row) => row.id}
    />
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByText("0–0 of 0")).toBeVisible()
    await expect(canvas.getByRole("button", { name: /next/i })).toBeDisabled()
  },
}

// Clicking Next advances the page; the range label and visible rows update.
export const Paginates: Story = {
  render: () => <Harness />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: /next/i }))
    await expect(canvas.getByText("11–20 of 23")).toBeVisible()
    await expect(canvas.getByText("user11@example.test")).toBeVisible()
    await expect(
      canvas.getByRole("button", { name: /previous/i }),
    ).toBeEnabled()
  },
}

// With onRowClick wired, clicking a row fires the callback with that row.
export const RowClick: Story = {
  render: (args) => <Harness onRowClick={args.onRowClick} />,
  args: { onRowClick: fn() },
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByText("user1@example.test"))
    await expect(args.onRowClick).toHaveBeenCalledWith(
      expect.objectContaining({ id: "s1" }),
    )
  },
}

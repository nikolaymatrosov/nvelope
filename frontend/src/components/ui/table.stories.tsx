import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "./table"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: Table,
  tags: ["ai-generated"],
} satisfies Meta<typeof Table>

export default meta
type Story = StoryObj<typeof meta>

const rows = [
  { name: "Welcome series", status: "Sent", recipients: 1204 },
  { name: "Monthly newsletter", status: "Draft", recipients: 3890 },
  { name: "Win-back", status: "Scheduled", recipients: 542 },
]

export const Default: Story = {
  render: () => (
    <Table className="w-[28rem]">
      <TableCaption>Recent campaigns</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>Campaign</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Recipients</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.name}>
            <TableCell>{row.name}</TableCell>
            <TableCell>{row.status}</TableCell>
            <TableCell className="text-right">{row.recipients}</TableCell>
          </TableRow>
        ))}
      </TableBody>
      <TableFooter>
        <TableRow>
          <TableCell colSpan={2}>Total</TableCell>
          <TableCell className="text-right">5,636</TableCell>
        </TableRow>
      </TableFooter>
    </Table>
  ),
}

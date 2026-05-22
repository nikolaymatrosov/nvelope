// A server-paged table consuming `{ items, total }` plus limit/offset
// (FR-016). Built on TanStack Table (headless) so column definitions, sorting,
// and selection can grow without reworking call sites. Pagination is manual —
// the server owns paging; this component just renders the current page.

import { useMemo } from "react"
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { ColumnDef } from "@tanstack/react-table"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"

export type { ColumnDef }

type DataTableProps<T> = {
  columns: Array<ColumnDef<T, unknown>>
  rows: Array<T>
  total: number
  limit: number
  offset: number
  onPageChange: (offset: number) => void
  getRowId: (row: T) => string
  onRowClick?: (row: T) => void
}

export function DataTable<T>({
  columns,
  rows,
  total,
  limit,
  offset,
  onPageChange,
  getRowId,
  onRowClick,
}: DataTableProps<T>) {
  const { t } = useTranslation()
  const table = useReactTable({
    data: rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId,
    manualPagination: true,
    pageCount: Math.max(1, Math.ceil(total / limit)),
  })

  const range = useMemo(() => {
    const from = total === 0 ? 0 : offset + 1
    const to = Math.min(offset + limit, total)
    return { from, to }
  }, [offset, limit, total])

  const canPrev = offset > 0
  const canNext = offset + limit < total

  return (
    <div className="flex flex-col gap-3">
      <div className="overflow-hidden rounded-lg border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => (
                  <TableHead key={header.id}>
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext(),
                        )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                className={onRowClick ? "cursor-pointer" : undefined}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          {t("table.range", {
            from: range.from,
            to: range.to,
            total,
          })}
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={!canPrev}
            onClick={() => onPageChange(Math.max(0, offset - limit))}
          >
            <ChevronLeftIcon /> {t("actions.previous")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!canNext}
            onClick={() => onPageChange(offset + limit)}
          >
            {t("actions.next")} <ChevronRightIcon />
          </Button>
        </div>
      </div>
    </div>
  )
}

import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import type { AuditRecord } from "@/lib/api-types"
import type { ColumnDef } from "@/components/common/data-table"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { DEFAULT_PAGE_SIZE } from "@/lib/api-types"
import { formatDateTime } from "@/lib/format"
import { AsyncState } from "@/components/common/async-state"
import { DataTable } from "@/components/common/data-table"

export const Route = createFileRoute("/t/$slug/audit/")({
  component: AuditView,
})

const columns: Array<ColumnDef<AuditRecord, unknown>> = [
  {
    id: "actor",
    header: "Actor",
    cell: ({ row }) => (
      <span className="text-sm">
        {row.original.ActorID}{" "}
        <span className="text-muted-foreground">({row.original.ActorKind})</span>
      </span>
    ),
  },
  { accessorKey: "Action", header: "Action" },
  {
    accessorKey: "Target",
    header: "Target",
    cell: ({ row }) => row.original.Target || "—",
  },
  {
    accessorKey: "CreatedAt",
    header: "When",
    cell: ({ row }) => formatDateTime(row.original.CreatedAt),
  },
]

export function AuditView() {
  const { slug } = Route.useParams()
  const [offset, setOffset] = useState(0)
  const limit = DEFAULT_PAGE_SIZE

  const auditQuery = useQuery({
    queryKey: queryKeys.audit(slug, limit, offset),
    queryFn: async () => (await api.auditTrail(slug, { limit, offset })).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Audit trail</h1>
        <p className="text-sm text-muted-foreground">
          Recent activity in this workspace.
        </p>
      </header>

      <AsyncState
        query={auditQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle="No activity yet"
        emptyMessage="Workspace actions will appear here."
      >
        {(data) => (
          <DataTable
            columns={columns}
            rows={data.records}
            total={data.total}
            limit={limit}
            offset={offset}
            onPageChange={setOffset}
            getRowId={(row) => row.ID}
          />
        )}
      </AsyncState>
    </div>
  )
}

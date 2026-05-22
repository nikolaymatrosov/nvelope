import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
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

export function AuditView() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("audit")
  const [offset, setOffset] = useState(0)
  const limit = DEFAULT_PAGE_SIZE

  const columns: Array<ColumnDef<AuditRecord, unknown>> = [
    {
      id: "actor",
      header: t("columns.actor"),
      cell: ({ row }) => (
        <span className="text-sm">
          {row.original.ActorID}{" "}
          <span className="text-muted-foreground">
            ({row.original.ActorKind})
          </span>
        </span>
      ),
    },
    { accessorKey: "Action", header: t("columns.action") },
    {
      accessorKey: "Target",
      header: t("columns.target"),
      cell: ({ row }) => row.original.Target || "—",
    },
    {
      accessorKey: "CreatedAt",
      header: t("columns.when"),
      cell: ({ row }) => formatDateTime(row.original.CreatedAt),
    },
  ]

  const auditQuery = useQuery({
    queryKey: queryKeys.audit(slug, limit, offset),
    queryFn: async () => (await api.auditTrail(slug, { limit, offset })).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("index.description")}
        </p>
      </header>

      <AsyncState
        query={auditQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle={t("index.emptyTitle")}
        emptyMessage={t("index.emptyMessage")}
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

import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { List } from "@/lib/api-types"
import type { ColumnDef } from "@/components/common/data-table"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { DEFAULT_PAGE_SIZE } from "@/lib/api-types"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"
import { DataTable } from "@/components/common/data-table"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/lists/")({ component: ListsView })

export function ListsView() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const { t } = useTranslation("lists")
  const { can } = usePermissions(slug)
  const canManage = can("lists:manage")
  const [offset, setOffset] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const limit = DEFAULT_PAGE_SIZE

  const columns: Array<ColumnDef<List, unknown>> = [
    { accessorKey: "Name", header: t("columns.name") },
    {
      accessorKey: "Description",
      header: t("columns.description"),
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {row.original.Description || "—"}
        </span>
      ),
    },
    {
      accessorKey: "Visibility",
      header: t("columns.visibility"),
      cell: ({ row }) => (
        <Badge variant="secondary">{row.original.Visibility}</Badge>
      ),
    },
    {
      accessorKey: "CreatedAt",
      header: t("columns.created"),
      cell: ({ row }) => formatDate(row.original.CreatedAt),
    },
  ]

  const listsQuery = useQuery({
    queryKey: queryKeys.listsPage(slug, limit, offset),
    queryFn: async () => (await api.listLists(slug, { limit, offset })).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("index.description")}
          </p>
        </div>
        {canManage && (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon /> {t("index.newList")}
          </Button>
        )}
      </header>

      <AsyncState
        query={listsQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle={t("index.emptyTitle")}
        emptyMessage={t("index.emptyMessage")}
        emptyAction={
          canManage ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> {t("index.newList")}
            </Button>
          ) : undefined
        }
      >
        {(data) => (
          <DataTable
            columns={columns}
            rows={data.lists}
            total={data.total}
            limit={limit}
            offset={offset}
            onPageChange={setOffset}
            getRowId={(row) => row.ID}
            onRowClick={(row) =>
              navigate({
                to: "/t/$slug/lists/$id",
                params: { slug, id: row.ID },
              })
            }
          />
        )}
      </AsyncState>

      <CreateListDialog
        slug={slug}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
    </div>
  )
}

function CreateListDialog({
  slug,
  open,
  onOpenChange,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["lists", "common"])

  const create = useMutation({
    mutationFn: (v: { name: string; description: string }) =>
      api.createList(slug, { name: v.name.trim(), description: v.description }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.lists(slug) })
      toast.success(t("create.success"))
      onOpenChange(false)
      form.reset()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: "", description: "" },
    onSubmit: async ({ value }) => {
      await create.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("create.title")}</DialogTitle>
          <DialogDescription>{t("create.description")}</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            form.handleSubmit()
          }}
        >
          <form.Field
            name="name"
            validators={{
              onBlur: compose(rules.required(t("form.nameRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("form.name")}
                required
                autoFocus
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <form.Field name="description">
            {(field) => (
              <FormField
                label={t("form.description")}
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
              />
            )}
          </form.Field>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common:actions.cancel")}
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? t("create.submitting") : t("create.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

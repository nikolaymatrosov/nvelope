import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { List, SegmentNode, Subscriber } from "@/lib/api-types"
import type { ColumnDef } from "@/components/common/data-table"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { DEFAULT_PAGE_SIZE } from "@/lib/api-types"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { AsyncState } from "@/components/common/async-state"
import { DataTable } from "@/components/common/data-table"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/lists/$id")({
  component: ListDetail,
})

function memberSegment(listId: string): SegmentNode {
  return { member: { listId } }
}

export function ListDetail() {
  const { slug, id } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { t } = useTranslation("lists")
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [offset, setOffset] = useState(0)
  const limit = DEFAULT_PAGE_SIZE

  const listQuery = useQuery({
    queryKey: queryKeys.list(slug, id),
    queryFn: async () => (await api.getList(slug, id)).data,
  })

  const subscribersQuery = useQuery({
    queryKey: queryKeys.subscribersQuery(slug, memberSegment(id), {
      limit,
      offset,
    }),
    queryFn: async () =>
      (
        await api.querySubscribers(slug, memberSegment(id), { limit, offset })
      ).data,
  })

  // updateList's generated request type requires visibility/optIn/tags
  // (OpenAPI `default:` becomes required in the generated TS type — see
  // lists/index.tsx). This form only edits name/description, so the current
  // list's other fields are echoed back unchanged rather than reset to a
  // hardcoded default.
  const update = useMutation({
    mutationFn: (v: {
      name: string
      description: string
      visibility: List["visibility"]
      optIn: List["optIn"]
      tags: List["tags"]
    }) => api.updateList(slug, id, v),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.lists(slug) })
      toast.success(t("detail.updateSuccess"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const remove = useMutation({
    mutationFn: () => api.deleteList(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.lists(slug) })
      toast.success(t("detail.deleteSuccess"))
      navigate({ to: "/t/$slug/lists", params: { slug } })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const columns: Array<ColumnDef<Subscriber, unknown>> = [
    { accessorKey: "email", header: t("subscriberColumns.email") },
    {
      accessorKey: "name",
      header: t("subscriberColumns.name"),
      cell: ({ row }) => row.original.name || "—",
    },
    {
      id: "state",
      header: t("subscriberColumns.subscription"),
      cell: ({ row }) => {
        const m = row.original.memberships.find((x) => x.listId === id)
        return <Badge variant="secondary">{m?.status ?? "—"}</Badge>
      },
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/lists"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          {t("detail.back")}
        </Link>
      </div>

      <AsyncState query={listQuery}>
        {(list) => (
          <EditListCard
            key={list.id}
            defaultName={list.name}
            defaultDescription={list.description}
            pending={update.isPending}
            onSave={(v) =>
              update.mutate({
                ...v,
                visibility: list.visibility,
                optIn: list.optIn,
                tags: list.tags,
              })
            }
            onDelete={() => setConfirmOpen(true)}
          />
        )}
      </AsyncState>

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">
          {t("detail.subscribersTitle")}
        </h2>
        <AsyncState
          query={subscribersQuery}
          isEmpty={(d) => d.total === 0}
          emptyTitle={t("detail.subscribersEmptyTitle")}
          emptyMessage={t("detail.subscribersEmptyMessage")}
        >
          {(data) => (
            <DataTable
              columns={columns}
              rows={data.items}
              total={data.total}
              limit={limit}
              offset={offset}
              onPageChange={setOffset}
              getRowId={(row) => row.id}
              onRowClick={(row) =>
                navigate({
                  to: "/t/$slug/subscribers/$id",
                  params: { slug, id: row.id },
                })
              }
            />
          )}
        </AsyncState>
      </section>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("detail.confirmDeleteTitle")}
        description={t("detail.confirmDeleteDescription")}
        confirmLabel={t("detail.deleteList")}
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </div>
  )
}

function EditListCard({
  defaultName,
  defaultDescription,
  pending,
  onSave,
  onDelete,
}: {
  defaultName: string
  defaultDescription: string
  pending: boolean
  onSave: (v: { name: string; description: string }) => void
  onDelete: () => void
}) {
  const { t } = useTranslation(["lists", "common"])
  const form = useForm({
    defaultValues: { name: defaultName, description: defaultDescription },
    onSubmit: ({ value }) => onSave(value),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("detail.cardTitle")}</CardTitle>
      </CardHeader>
      <CardContent>
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
          <div className="flex justify-between">
            <Button type="submit" disabled={pending}>
              {pending ? t("common:actions.saving") : t("detail.saveChanges")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={onDelete}
            >
              <Trash2Icon /> {t("detail.deleteList")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

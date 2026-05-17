import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import type { Node, Subscriber } from "@/lib/api-types"
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

function memberSegment(listId: string): Node {
  return { Member: { ListID: listId, Status: "subscribed" } }
}

export function ListDetail() {
  const { slug, id } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [offset, setOffset] = useState(0)
  const limit = DEFAULT_PAGE_SIZE

  const listQuery = useQuery({
    queryKey: queryKeys.list(slug, id),
    queryFn: async () => (await api.getList(slug, id)).data.list,
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

  const update = useMutation({
    mutationFn: (v: { name: string; description: string }) =>
      api.updateList(slug, id, { name: v.name.trim(), description: v.description }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.lists(slug) })
      toast.success("List updated.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const remove = useMutation({
    mutationFn: () => api.deleteList(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.lists(slug) })
      toast.success("List deleted.")
      navigate({ to: "/t/$slug/lists", params: { slug } })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const columns: Array<ColumnDef<Subscriber, unknown>> = [
    { accessorKey: "Email", header: "Email" },
    {
      accessorKey: "Name",
      header: "Name",
      cell: ({ row }) => row.original.Name || "—",
    },
    {
      id: "state",
      header: "Subscription",
      cell: ({ row }) => {
        const m = row.original.Memberships.find((x) => x.ListID === id)
        return <Badge variant="secondary">{m?.Status ?? "—"}</Badge>
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
          ← Lists
        </Link>
      </div>

      <AsyncState query={listQuery}>
        {(list) => (
          <EditListCard
            key={list.ID}
            defaultName={list.Name}
            defaultDescription={list.Description}
            pending={update.isPending}
            onSave={(v) => update.mutate(v)}
            onDelete={() => setConfirmOpen(true)}
          />
        )}
      </AsyncState>

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">Subscribers in this list</h2>
        <AsyncState
          query={subscribersQuery}
          isEmpty={(d) => d.total === 0}
          emptyTitle="No subscribers yet"
          emptyMessage="No subscribers are subscribed to this list."
        >
          {(data) => (
            <DataTable
              columns={columns}
              rows={data.subscribers}
              total={data.total}
              limit={limit}
              offset={offset}
              onPageChange={setOffset}
              getRowId={(row) => row.ID}
              onRowClick={(row) =>
                navigate({
                  to: "/t/$slug/subscribers/$id",
                  params: { slug, id: row.ID },
                })
              }
            />
          )}
        </AsyncState>
      </section>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete this list?"
        description="The list will be removed. Subscribers are not deleted."
        confirmLabel="Delete list"
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
  const form = useForm({
    defaultValues: { name: defaultName, description: defaultDescription },
    onSubmit: ({ value }) => onSave(value),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>List details</CardTitle>
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
            validators={{ onBlur: compose(rules.required("Enter a list name.")) }}
          >
            {(field) => (
              <FormField
                label="Name"
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
                label="Description"
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
              />
            )}
          </form.Field>
          <div className="flex justify-between">
            <Button type="submit" disabled={pending}>
              {pending ? "Saving…" : "Save changes"}
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={onDelete}
            >
              <Trash2Icon /> Delete list
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

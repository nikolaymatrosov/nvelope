import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { toast } from "sonner"
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

const columns: Array<ColumnDef<List, unknown>> = [
  { accessorKey: "Name", header: "Name" },
  {
    accessorKey: "Description",
    header: "Description",
    cell: ({ row }) => (
      <span className="text-muted-foreground">
        {row.original.Description || "—"}
      </span>
    ),
  },
  {
    accessorKey: "Visibility",
    header: "Visibility",
    cell: ({ row }) => <Badge variant="secondary">{row.original.Visibility}</Badge>,
  },
  {
    accessorKey: "CreatedAt",
    header: "Created",
    cell: ({ row }) => formatDate(row.original.CreatedAt),
  },
]

export function ListsView() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const { can } = usePermissions(slug)
  const canManage = can("lists:manage")
  const [offset, setOffset] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const limit = DEFAULT_PAGE_SIZE

  const listsQuery = useQuery({
    queryKey: queryKeys.listsPage(slug, limit, offset),
    queryFn: async () => (await api.listLists(slug, { limit, offset })).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Lists</h1>
          <p className="text-sm text-muted-foreground">
            Organise subscribers into lists.
          </p>
        </div>
        {canManage && (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon /> New list
          </Button>
        )}
      </header>

      <AsyncState
        query={listsQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle="No lists yet"
        emptyMessage="Create your first list to start grouping subscribers."
        emptyAction={
          canManage ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> New list
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

  const create = useMutation({
    mutationFn: (v: { name: string; description: string }) =>
      api.createList(slug, { name: v.name.trim(), description: v.description }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.lists(slug) })
      toast.success("List created.")
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
          <DialogTitle>New list</DialogTitle>
          <DialogDescription>
            Give the list a name. You can add subscribers afterwards.
          </DialogDescription>
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
            validators={{ onBlur: compose(rules.required("Enter a list name.")) }}
          >
            {(field) => (
              <FormField
                label="Name"
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
                label="Description"
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
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Creating…" : "Create list"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

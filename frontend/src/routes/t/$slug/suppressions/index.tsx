import { Link, createFileRoute } from "@tanstack/react-router"
import { useDeferredValue, useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, SettingsIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import type { SuppressionEntry, SuppressionReason } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isNotFound, isValidation } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/suppressions/")({
  component: SuppressionsView,
})

const REASON_LABEL: Record<SuppressionReason, string> = {
  hard_bounce: "Hard bounce",
  complaint: "Complaint",
  manual: "Manual",
}

const REASON_VARIANT: Record<
  SuppressionReason,
  "default" | "secondary" | "destructive"
> = {
  hard_bounce: "destructive",
  complaint: "destructive",
  manual: "secondary",
}

function ReasonBadge({ reason }: { reason: SuppressionReason }) {
  return (
    <Badge variant={REASON_VARIANT[reason]}>{REASON_LABEL[reason]}</Badge>
  )
}

export function SuppressionsView() {
  const { slug } = Route.useParams()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)
  const canManage = can("sending:manage")

  const [reason, setReason] = useState<"" | SuppressionReason>("")
  const [search, setSearch] = useState("")
  const [createOpen, setCreateOpen] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<string | null>(null)

  const email = useDeferredValue(search).trim()
  const filters = { reason: reason || undefined, email: email || undefined }

  const query = useInfiniteQuery({
    queryKey: queryKeys.suppressions(slug, {
      reason: filters.reason,
      email: filters.email,
    }),
    queryFn: async ({ pageParam }) =>
      (
        await api.suppressions.list(slug, {
          cursor: pageParam,
          reason: filters.reason,
          email: filters.email,
        })
      ).data,
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    retry: false,
  })

  const items = query.data?.pages.flatMap((p) => p.items) ?? []
  const hasFilter = Boolean(filters.reason || filters.email)

  const listState = {
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    data: query.data ? items : undefined,
    refetch: () => query.refetch(),
  }

  function invalidate() {
    return queryClient.invalidateQueries({
      queryKey: ["t", slug, "suppressions"],
    })
  }

  const remove = useMutation({
    mutationFn: (target: string) => api.suppressions.remove(slug, target),
    onSuccess: async () => {
      await invalidate()
      toast.success("Address removed from the suppression list.")
    },
    onError: async (e) => {
      // A concurrent removal already deleted the entry — reconcile silently.
      if (isNotFound(e)) {
        await invalidate()
        return
      }
      toast.error(errorMessage(e))
    },
    onSettled: () => setRemoveTarget(null),
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Suppression list</h1>
          <p className="text-sm text-muted-foreground">
            Addresses that will not be mailed for this workspace.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" asChild>
            <Link to="/t/$slug/suppressions/settings" params={{ slug }}>
              <SettingsIcon /> Bounce settings
            </Link>
          </Button>
          {canManage && (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> Add address
            </Button>
          )}
        </div>
      </header>

      <div className="flex flex-wrap items-center gap-3">
        <Select
          value={reason || "all"}
          onValueChange={(v) =>
            setReason(v === "all" ? "" : (v as SuppressionReason))
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter by reason">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All reasons</SelectItem>
            <SelectItem value="hard_bounce">Hard bounce</SelectItem>
            <SelectItem value="complaint">Complaint</SelectItem>
            <SelectItem value="manual">Manual</SelectItem>
          </SelectContent>
        </Select>
        <Input
          className="w-64"
          placeholder="Search by address"
          aria-label="Search by address"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <AsyncState
        query={listState}
        isEmpty={(d) => d.length === 0}
        emptyTitle={hasFilter ? "No matching addresses" : "No suppressed addresses"}
        emptyMessage={
          hasFilter
            ? "No suppressed addresses match the current filter."
            : "Bounces and complaints are added here automatically. You can also add an address manually."
        }
        emptyAction={
          canManage && !hasFilter ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> Add address
            </Button>
          ) : undefined
        }
      >
        {(entries) => (
          <div className="flex flex-col gap-2">
            {entries.map((entry) => (
              <SuppressionRow
                key={entry.email}
                entry={entry}
                canManage={canManage}
                onRemove={() => setRemoveTarget(entry.email)}
              />
            ))}
            {query.hasNextPage && (
              <Button
                variant="outline"
                className="self-center"
                disabled={query.isFetchingNextPage}
                onClick={() => query.fetchNextPage()}
              >
                {query.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            )}
          </div>
        )}
      </AsyncState>

      <AddAddressDialog
        slug={slug}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(o) => !o && setRemoveTarget(null)}
        title="Remove from suppression list?"
        description={
          <>
            <span className="font-medium">{removeTarget}</span> will become
            mailable again and may receive future campaigns.
          </>
        }
        confirmLabel="Remove"
        busy={remove.isPending}
        onConfirm={() => removeTarget && remove.mutate(removeTarget)}
      />
    </div>
  )
}

function SuppressionRow({
  entry,
  canManage,
  onRemove,
}: {
  entry: SuppressionEntry
  canManage: boolean
  onRemove: () => void
}) {
  return (
    <div className="flex items-center gap-3 rounded-lg border p-3">
      <div className="flex-1">
        <p className="text-sm font-medium">{entry.email}</p>
        <p className="text-xs text-muted-foreground">
          Suppressed {formatDate(entry.suppressedAt)}
          {entry.note ? ` · ${entry.note}` : ""}
        </p>
      </div>
      <ReasonBadge reason={entry.reason} />
      {canManage && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Remove ${entry.email}`}
          onClick={onRemove}
        >
          <Trash2Icon />
        </Button>
      )}
    </div>
  )
}

function AddAddressDialog({
  slug,
  open,
  onOpenChange,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [serverError, setServerError] = useState<string | undefined>()

  const add = useMutation({
    mutationFn: (value: string) => api.suppressions.add(slug, value.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["t", slug, "suppressions"],
      })
      toast.success("Address added to the suppression list.")
      onOpenChange(false)
      form.reset()
    },
    onError: (e) => {
      if (isValidation(e)) {
        setServerError(errorMessage(e))
        return
      }
      toast.error(errorMessage(e))
    },
  })

  const form = useForm({
    defaultValues: { email: "" },
    onSubmit: async ({ value }) => {
      setServerError(undefined)
      await add.mutateAsync(value.email).catch(() => {})
    },
  })

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          setServerError(undefined)
          form.reset()
        }
        onOpenChange(o)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add an address</DialogTitle>
          <DialogDescription>
            The address will be skipped on all future sends until you remove it.
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
            name="email"
            validators={{
              onBlur: compose(
                rules.required("Enter an email address."),
                rules.email(),
              ),
            }}
          >
            {(field) => (
              <FormField
                label="Email address"
                required
                autoFocus
                type="email"
                placeholder="person@example.com"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => {
                  setServerError(undefined)
                  field.handleChange(e.target.value)
                }}
                error={fieldError(field.state.meta.errors) ?? serverError}
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
            <Button type="submit" disabled={add.isPending}>
              {add.isPending ? "Adding…" : "Add address"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

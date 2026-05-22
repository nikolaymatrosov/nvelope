import { Link, createFileRoute } from "@tanstack/react-router"
import { useDeferredValue, useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, SettingsIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
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

const REASON_VARIANT: Record<
  SuppressionReason,
  "default" | "secondary" | "destructive"
> = {
  hard_bounce: "destructive",
  complaint: "destructive",
  manual: "secondary",
}

const REASON_KEY = {
  hard_bounce: "reasons.hardBounce",
  complaint: "reasons.complaint",
  manual: "reasons.manual",
} as const satisfies Record<SuppressionReason, string>

function ReasonBadge({ reason }: { reason: SuppressionReason }) {
  const { t } = useTranslation("suppressions")
  return (
    <Badge variant={REASON_VARIANT[reason]}>{t(REASON_KEY[reason])}</Badge>
  )
}

export function SuppressionsView() {
  const { slug } = Route.useParams()
  const queryClient = useQueryClient()
  const { t } = useTranslation("suppressions")
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
      toast.success(t("remove.successToast"))
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
          <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("index.description")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" asChild>
            <Link to="/t/$slug/suppressions/settings" params={{ slug }}>
              <SettingsIcon /> {t("index.bounceSettings")}
            </Link>
          </Button>
          {canManage && (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> {t("index.addAddress")}
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
          <SelectTrigger className="w-48" aria-label={t("index.filterReasonAria")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("index.allReasons")}</SelectItem>
            <SelectItem value="hard_bounce">
              {t("reasons.hardBounce")}
            </SelectItem>
            <SelectItem value="complaint">{t("reasons.complaint")}</SelectItem>
            <SelectItem value="manual">{t("reasons.manual")}</SelectItem>
          </SelectContent>
        </Select>
        <Input
          className="w-64"
          placeholder={t("index.searchPlaceholder")}
          aria-label={t("index.searchAria")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <AsyncState
        query={listState}
        isEmpty={(d) => d.length === 0}
        emptyTitle={
          hasFilter
            ? t("index.emptyFilteredTitle")
            : t("index.emptyTitle")
        }
        emptyMessage={
          hasFilter
            ? t("index.emptyFilteredMessage")
            : t("index.emptyMessage")
        }
        emptyAction={
          canManage && !hasFilter ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> {t("index.addAddress")}
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
                {query.isFetchingNextPage
                  ? t("index.loadingMore")
                  : t("index.loadMore")}
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
        title={t("remove.confirmTitle")}
        description={
          <>
            <span className="font-medium">{removeTarget}</span>{" "}
            {t("remove.confirmDescription")}
          </>
        }
        confirmLabel={t("remove.confirmLabel")}
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
  const { t } = useTranslation("suppressions")
  return (
    <div className="flex items-center gap-3 rounded-lg border p-3">
      <div className="flex-1">
        <p className="text-sm font-medium">{entry.email}</p>
        <p className="text-xs text-muted-foreground">
          {t("row.suppressedAt", { date: formatDate(entry.suppressedAt) })}
          {entry.note ? ` · ${entry.note}` : ""}
        </p>
      </div>
      <ReasonBadge reason={entry.reason} />
      {canManage && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t("row.removeAria", { email: entry.email })}
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
  const { t } = useTranslation(["suppressions", "common"])
  const [serverError, setServerError] = useState<string | undefined>()

  const add = useMutation({
    mutationFn: (value: string) => api.suppressions.add(slug, value.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["t", slug, "suppressions"],
      })
      toast.success(t("addDialog.successToast"))
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
          <DialogTitle>{t("addDialog.title")}</DialogTitle>
          <DialogDescription>
            {t("addDialog.description")}
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
                rules.required(t("addDialog.emailRequired")),
                rules.email(),
              ),
            }}
          >
            {(field) => (
              <FormField
                label={t("addDialog.emailLabel")}
                required
                autoFocus
                type="email"
                placeholder={t("addDialog.emailPlaceholder")}
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
              {t("common:actions.cancel")}
            </Button>
            <Button type="submit" disabled={add.isPending}>
              {add.isPending
                ? t("addDialog.submitting")
                : t("addDialog.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

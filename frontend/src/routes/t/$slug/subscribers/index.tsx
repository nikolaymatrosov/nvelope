import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, SearchIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { Node, Subscriber } from "@/lib/api-types"
import type { ColumnDef } from "@/components/common/data-table"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isConflict } from "@/lib/errors"
import { DEFAULT_PAGE_SIZE } from "@/lib/api-types"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Checkbox } from "@/components/ui/checkbox"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
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
import { JsonAttributeEditor } from "@/components/common/json-attribute-editor"
import { SegmentBuilder, emptyGroup } from "@/components/common/segment-builder"

export const Route = createFileRoute("/t/$slug/subscribers/")({
  component: SubscribersView,
})

function useSubscriberColumns(): Array<ColumnDef<Subscriber, unknown>> {
  const { t } = useTranslation("subscribers")
  return [
    { accessorKey: "Email", header: t("list.columns.email") },
    {
      accessorKey: "Name",
      header: t("list.columns.name"),
      cell: ({ row }) => row.original.Name || "—",
    },
    {
      accessorKey: "State",
      header: t("list.columns.state"),
      cell: ({ row }) => (
        <Badge variant="secondary">{row.original.State}</Badge>
      ),
    },
    {
      id: "lists",
      header: t("list.columns.lists"),
      cell: ({ row }) => row.original.Memberships.length,
    },
  ]
}

export function SubscribersView() {
  const { slug } = Route.useParams()
  const { can } = usePermissions(slug)
  const { t } = useTranslation("subscribers")
  const [createOpen, setCreateOpen] = useState(false)

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">{t("list.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("list.description")}
          </p>
        </div>
        {can("subscribers:manage") && (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon /> {t("list.newSubscriber")}
          </Button>
        )}
      </header>

      <Tabs defaultValue="search">
        <TabsList>
          <TabsTrigger value="search">{t("list.tabs.search")}</TabsTrigger>
          <TabsTrigger value="segment">{t("list.tabs.segment")}</TabsTrigger>
        </TabsList>
        <TabsContent value="search">
          <SearchPanel slug={slug} />
        </TabsContent>
        <TabsContent value="segment">
          <SegmentPanel slug={slug} />
        </TabsContent>
      </Tabs>

      <CreateSubscriberDialog
        slug={slug}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
    </div>
  )
}

function SearchPanel({ slug }: { slug: string }) {
  const navigate = useNavigate()
  const { t } = useTranslation("subscribers")
  const columns = useSubscriberColumns()
  const [term, setTerm] = useState("")
  const [query, setQuery] = useState("")
  const [offset, setOffset] = useState(0)
  const limit = DEFAULT_PAGE_SIZE

  const subscribersQuery = useQuery({
    queryKey: queryKeys.subscribersSearch(slug, query, limit, offset),
    queryFn: async () =>
      (await api.searchSubscribers(slug, query, { limit, offset })).data,
  })

  return (
    <div className="flex flex-col gap-4 pt-4">
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setOffset(0)
          setQuery(term)
        }}
      >
        <Input
          placeholder={t("list.search.placeholder")}
          value={term}
          onChange={(e) => setTerm(e.target.value)}
        />
        <Button type="submit" variant="outline">
          <SearchIcon /> {t("list.search.submit")}
        </Button>
      </form>
      <AsyncState
        query={subscribersQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle={t("list.search.emptyTitle")}
        emptyMessage={t("list.search.emptyMessage")}
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
    </div>
  )
}

function SegmentPanel({ slug }: { slug: string }) {
  const navigate = useNavigate()
  const { t } = useTranslation("subscribers")
  const columns = useSubscriberColumns()
  const [draft, setDraft] = useState<Node>(emptyGroup())
  const [active, setActive] = useState<Node | null>(null)
  const [offset, setOffset] = useState(0)
  const limit = DEFAULT_PAGE_SIZE

  const segmentQuery = useQuery({
    queryKey: queryKeys.subscribersQuery(slug, active, { limit, offset }),
    queryFn: async () =>
      (await api.querySubscribers(slug, active as Node, { limit, offset })).data,
    enabled: active !== null,
  })

  return (
    <div className="flex flex-col gap-4 pt-4">
      <SegmentBuilder value={draft} onChange={setDraft} />
      <div>
        <Button
          onClick={() => {
            setOffset(0)
            setActive(draft)
          }}
        >
          {t("list.segment.run")}
        </Button>
      </div>
      {active !== null && (
        <AsyncState
          query={segmentQuery}
          isEmpty={(d) => d.total === 0}
          emptyTitle={t("list.segment.emptyTitle")}
          emptyMessage={t("list.segment.emptyMessage")}
        >
          {(data) => (
            <div className="flex flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                {t("list.segment.matchCount", { count: data.total })}
              </p>
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
            </div>
          )}
        </AsyncState>
      )}
    </div>
  )
}

function CreateSubscriberDialog({
  slug,
  open,
  onOpenChange,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["subscribers", "common"])
  const [attributes, setAttributes] = useState<Record<string, unknown>>({})
  const [attrsValid, setAttrsValid] = useState(true)
  const [listIds, setListIds] = useState<Array<string>>([])
  const [emailTaken, setEmailTaken] = useState(false)

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 100, offset: 0 })).data.lists,
    enabled: open,
  })

  const create = useMutation({
    mutationFn: (v: { email: string; name: string }) =>
      api.createSubscriber(slug, {
        email: v.email.trim(),
        name: v.name.trim(),
        attributes,
        list_ids: listIds,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.subscribers(slug),
      })
      toast.success(t("create.success"))
      reset()
      onOpenChange(false)
    },
    onError: (e) => {
      if (isConflict(e)) {
        setEmailTaken(true)
        return
      }
      toast.error(errorMessage(e))
    },
  })

  const form = useForm({
    defaultValues: { email: "", name: "" },
    onSubmit: async ({ value }) => {
      setEmailTaken(false)
      if (!attrsValid) return
      await create.mutateAsync(value).catch(() => {})
    },
  })

  function reset() {
    form.reset()
    setAttributes({})
    setListIds([])
    setEmailTaken(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset()
        onOpenChange(o)
      }}
    >
      <DialogContent className="max-h-[90svh] overflow-y-auto">
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
            name="email"
            validators={{ onBlur: compose(rules.required(), rules.email()) }}
          >
            {(field) => (
              <FormField
                label={t("create.email")}
                type="email"
                required
                autoFocus
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => {
                  setEmailTaken(false)
                  field.handleChange(e.target.value)
                }}
                error={
                  emailTaken
                    ? t("create.emailTaken")
                    : fieldError(field.state.meta.errors)
                }
              />
            )}
          </form.Field>
          <form.Field name="name">
            {(field) => (
              <FormField
                label={t("create.name")}
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
              />
            )}
          </form.Field>
          <JsonAttributeEditor
            value={attributes}
            onChange={setAttributes}
            onValidityChange={setAttrsValid}
          />
          <div className="flex flex-col gap-2">
            <Label>{t("create.addToLists")}</Label>
            {listsQuery.data && listsQuery.data.length > 0 ? (
              <div className="flex flex-col gap-2 rounded-lg border p-3">
                {listsQuery.data.map((list) => (
                  <label
                    key={list.ID}
                    className="flex items-center gap-2 text-sm"
                  >
                    <Checkbox
                      checked={listIds.includes(list.ID)}
                      onCheckedChange={(checked) =>
                        setListIds((prev) =>
                          checked
                            ? [...prev, list.ID]
                            : prev.filter((x) => x !== list.ID),
                        )
                      }
                    />
                    {list.Name}
                  </label>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                {t("create.noLists")}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common:actions.cancel")}
            </Button>
            <Button type="submit" disabled={create.isPending || !attrsValid}>
              {create.isPending ? t("create.submitting") : t("create.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

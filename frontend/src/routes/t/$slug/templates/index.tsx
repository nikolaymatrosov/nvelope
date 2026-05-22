import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { TemplateKind, TemplateView } from "@/lib/api-types"
import type { ColumnDef } from "@/components/common/data-table"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { DEFAULT_PAGE_SIZE } from "@/lib/api-types"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
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
import { DataTable } from "@/components/common/data-table"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/templates/")({
  component: TemplatesView,
})

export function TemplatesView() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { t } = useTranslation("templates")
  const { can } = usePermissions(slug)
  const canManage = can("campaigns:manage")
  const [offset, setOffset] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<TemplateView | null>(null)
  const limit = DEFAULT_PAGE_SIZE

  const templatesQuery = useQuery({
    queryKey: queryKeys.templatesPage(slug, limit, offset),
    queryFn: async () => (await api.listTemplates(slug, { limit, offset })).data,
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteTemplate(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.templates(slug),
      })
      toast.success(t("list.deleteSuccess"))
      setDeleting(null)
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      setDeleting(null)
    },
  })

  const columns: Array<ColumnDef<TemplateView, unknown>> = [
    { accessorKey: "name", header: t("list.columns.name") },
    {
      accessorKey: "kind",
      header: t("list.columns.kind"),
      cell: ({ row }) => (
        <Badge variant="secondary">{t(`kind.${row.original.kind}`)}</Badge>
      ),
    },
    {
      accessorKey: "subject",
      header: t("list.columns.subject"),
      cell: ({ row }) => (
        <span className="text-muted-foreground">{row.original.subject}</span>
      ),
    },
    {
      accessorKey: "updated_at",
      header: t("list.columns.updated"),
      cell: ({ row }) => formatDate(row.original.updated_at),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) =>
        canManage ? (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("list.deleteAction")}
            onClick={(e) => {
              e.stopPropagation()
              setDeleting(row.original)
            }}
          >
            <Trash2Icon />
          </Button>
        ) : null,
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">{t("list.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("list.description")}
          </p>
        </div>
        {canManage && (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon /> {t("list.newTemplate")}
          </Button>
        )}
      </header>

      <AsyncState
        query={templatesQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle={t("list.emptyTitle")}
        emptyMessage={t("list.emptyMessage")}
        emptyAction={
          canManage ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> {t("list.newTemplate")}
            </Button>
          ) : undefined
        }
      >
        {(data) => (
          <DataTable
            columns={columns}
            rows={data.templates}
            total={data.total}
            limit={limit}
            offset={offset}
            onPageChange={setOffset}
            getRowId={(row) => row.id}
            onRowClick={(row) =>
              navigate({
                to: "/t/$slug/templates/$id",
                params: { slug, id: row.id },
              })
            }
          />
        )}
      </AsyncState>

      <CreateTemplateDialog
        slug={slug}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={t("list.confirmDelete.title")}
        description={t("list.confirmDelete.description")}
        confirmLabel={t("list.confirmDelete.confirm")}
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </div>
  )
}

function CreateTemplateDialog({
  slug,
  open,
  onOpenChange,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["templates", "common"])
  const [kind, setKind] = useState<TemplateKind>("campaign")

  const create = useMutation({
    mutationFn: (v: {
      name: string
      subject: string
      bodyHtml: string
      bodyText: string
    }) =>
      api.createTemplate(slug, {
        name: v.name.trim(),
        kind,
        subject: v.subject,
        body_html: v.bodyHtml,
        body_text: v.bodyText,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.templates(slug),
      })
      toast.success(t("create.success"))
      onOpenChange(false)
      setKind("campaign")
      form.reset()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: "", subject: "", bodyHtml: "", bodyText: "" },
    onSubmit: async ({ value }) => {
      await create.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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
            name="name"
            validators={{
              onBlur: compose(rules.required(t("create.nameRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("create.nameLabel")}
                required
                autoFocus
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <div className="flex flex-col gap-1.5">
            <Label>{t("create.typeLabel")}</Label>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as TemplateKind)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="campaign">{t("kind.campaign")}</SelectItem>
                <SelectItem value="transactional">
                  {t("kind.transactional")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <form.Field
            name="subject"
            validators={{
              onBlur: compose(rules.required(t("create.subjectRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("create.subjectLabel")}
                required
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <form.Field name="bodyHtml">
            {(field) => (
              <FormField label={t("create.htmlBodyLabel")}>
                <Textarea
                  rows={6}
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                />
              </FormField>
            )}
          </form.Field>
          <form.Field name="bodyText">
            {(field) => (
              <FormField label={t("create.textBodyLabel")}>
                <Textarea
                  rows={4}
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                />
              </FormField>
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

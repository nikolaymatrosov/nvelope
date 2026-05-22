import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { CampaignStatus, CampaignView } from "@/lib/api-types"
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
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/campaigns/")({
  component: CampaignsView,
})

const STATUS_VARIANT: Record<
  CampaignStatus,
  "default" | "secondary" | "destructive" | "outline"
> = {
  draft: "secondary",
  running: "default",
  paused: "outline",
  finished: "secondary",
  cancelled: "destructive",
}

export function CampaignStatusBadge({ status }: { status: CampaignStatus }) {
  const { t } = useTranslation("campaigns")
  return (
    <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`)}</Badge>
  )
}

function ProgressLabel({ campaign }: { campaign: CampaignView }) {
  const { t } = useTranslation("campaigns")
  if (campaign.status === "draft") return <>{t("list.progress.draft")}</>
  const remaining = Math.max(
    0,
    campaign.recipient_count - campaign.sent_count - campaign.failed_count,
  )
  return (
    <>
      {t("list.progress.summary", {
        sent: campaign.sent_count,
        failed: campaign.failed_count,
        remaining,
      })}
    </>
  )
}

export function CampaignsView() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const { t } = useTranslation("campaigns")
  const { can } = usePermissions(slug)
  const canManage = can("campaigns:manage")
  const [offset, setOffset] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const limit = DEFAULT_PAGE_SIZE

  const campaignsQuery = useQuery({
    queryKey: queryKeys.campaignsPage(slug, limit, offset),
    queryFn: async () => (await api.listCampaigns(slug, { limit, offset })).data,
  })

  const columns: Array<ColumnDef<CampaignView, unknown>> = [
    { accessorKey: "name", header: t("list.columns.name") },
    {
      accessorKey: "status",
      header: t("list.columns.status"),
      cell: ({ row }) => <CampaignStatusBadge status={row.original.status} />,
    },
    {
      id: "progress",
      header: t("list.columns.progress"),
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          <ProgressLabel campaign={row.original} />
        </span>
      ),
    },
    {
      accessorKey: "created_at",
      header: t("list.columns.created"),
      cell: ({ row }) => formatDate(row.original.created_at),
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
            <PlusIcon /> {t("list.newCampaign")}
          </Button>
        )}
      </header>

      <AsyncState
        query={campaignsQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle={t("list.emptyTitle")}
        emptyMessage={t("list.emptyMessage")}
        emptyAction={
          canManage ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> {t("list.newCampaign")}
            </Button>
          ) : undefined
        }
      >
        {(data) => (
          <DataTable
            columns={columns}
            rows={data.campaigns}
            total={data.total}
            limit={limit}
            offset={offset}
            onPageChange={setOffset}
            getRowId={(row) => row.id}
            onRowClick={(row) =>
              navigate({
                to: "/t/$slug/campaigns/$id",
                params: { slug, id: row.id },
              })
            }
          />
        )}
      </AsyncState>

      <CreateCampaignDialog
        slug={slug}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
    </div>
  )
}

function CreateCampaignDialog({
  slug,
  open,
  onOpenChange,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { t } = useTranslation(["campaigns", "common"])
  const [templateId, setTemplateId] = useState<string>("")

  const templatesQuery = useQuery({
    queryKey: queryKeys.templates(slug),
    queryFn: async () => (await api.listTemplates(slug)).data.templates,
    enabled: open,
  })
  const campaignTemplates = (templatesQuery.data ?? []).filter(
    (tpl) => tpl.kind === "campaign",
  )

  const create = useMutation({
    mutationFn: (name: string) =>
      api.createCampaign(slug, {
        name: name.trim(),
        template_id: templateId || undefined,
        subject: "",
        body_html: "",
        body_text: "",
        from_name: "",
        from_local_part: "",
        list_ids: [],
      }),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.campaigns(slug),
      })
      toast.success(t("create.success"))
      onOpenChange(false)
      setTemplateId("")
      form.reset()
      navigate({
        to: "/t/$slug/campaigns/$id",
        params: { slug, id: res.data.id },
      })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: "" },
    onSubmit: async ({ value }) => {
      await create.mutateAsync(value.name).catch(() => {})
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
            <Label>{t("create.templateLabel")}</Label>
            <Select
              value={templateId || "none"}
              onValueChange={(v) => setTemplateId(v === "none" ? "" : v)}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("create.noTemplate")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">{t("create.noTemplate")}</SelectItem>
                {campaignTemplates.map((tpl) => (
                  <SelectItem key={tpl.id} value={tpl.id}>
                    {tpl.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
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

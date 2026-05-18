import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { toast } from "sonner"
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
  return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>
}

function progressLabel(c: CampaignView): string {
  if (c.status === "draft") return "—"
  const remaining = Math.max(
    0,
    c.recipient_count - c.sent_count - c.failed_count,
  )
  return `${c.sent_count} sent · ${c.failed_count} failed · ${remaining} left`
}

export function CampaignsView() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
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
    { accessorKey: "name", header: "Name" },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => <CampaignStatusBadge status={row.original.status} />,
    },
    {
      id: "progress",
      header: "Progress",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {progressLabel(row.original)}
        </span>
      ),
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => formatDate(row.original.created_at),
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Campaigns</h1>
          <p className="text-sm text-muted-foreground">
            Author and send campaigns to your lists.
          </p>
        </div>
        {canManage && (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon /> New campaign
          </Button>
        )}
      </header>

      <AsyncState
        query={campaignsQuery}
        isEmpty={(d) => d.total === 0}
        emptyTitle="No campaigns yet"
        emptyMessage="Create your first campaign to start sending."
        emptyAction={
          canManage ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> New campaign
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
  const [templateId, setTemplateId] = useState<string>("")

  const templatesQuery = useQuery({
    queryKey: queryKeys.templates(slug),
    queryFn: async () => (await api.listTemplates(slug)).data.templates,
    enabled: open,
  })
  const campaignTemplates = (templatesQuery.data ?? []).filter(
    (t) => t.kind === "campaign",
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
      toast.success("Campaign created.")
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
          <DialogTitle>New campaign</DialogTitle>
          <DialogDescription>
            Name the campaign. You can optionally start from a campaign
            template, then edit its content.
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
            validators={{
              onBlur: compose(rules.required("Enter a campaign name.")),
            }}
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
          <div className="flex flex-col gap-1.5">
            <Label>Start from a template (optional)</Label>
            <Select
              value={templateId || "none"}
              onValueChange={(v) => setTemplateId(v === "none" ? "" : v)}
            >
              <SelectTrigger>
                <SelectValue placeholder="No template" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">No template</SelectItem>
                {campaignTemplates.map((t) => (
                  <SelectItem key={t.id} value={t.id}>
                    {t.name}
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
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Creating…" : "Create campaign"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

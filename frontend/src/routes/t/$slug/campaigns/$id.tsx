import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { CampaignStatusBadge } from "./index"
import type { CampaignView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import {
  campaignProgress,
  isAutoPaused,
  useCampaign,
} from "@/hooks/use-campaign"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Checkbox } from "@/components/ui/checkbox"
import { Progress } from "@/components/ui/progress"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { AsyncState } from "@/components/common/async-state"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/campaigns/$id")({
  component: CampaignDetail,
})

export function CampaignDetail() {
  const { slug, id } = Route.useParams()
  const { can } = usePermissions(slug)
  const canManage = can("campaigns:manage")
  const { query } = useCampaign(slug, id)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/campaigns"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          ← Campaigns
        </Link>
      </div>

      <AsyncState query={query}>
        {(campaign) => (
          <CampaignEditor
            key={campaign.id}
            slug={slug}
            campaign={campaign}
            canManage={canManage}
          />
        )}
      </AsyncState>
    </div>
  )
}

function CampaignEditor({
  slug,
  campaign,
  canManage,
}: {
  slug: string
  campaign: CampaignView
  canManage: boolean
}) {
  const queryClient = useQueryClient()
  const isDraft = campaign.status === "draft"

  const [domainId, setDomainId] = useState(campaign.sending_domain_id ?? "")
  const [listIds, setListIds] = useState<Array<string>>(campaign.list_ids)
  const [confirmStart, setConfirmStart] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false)

  const domainsQuery = useQuery({
    queryKey: queryKeys.sendingDomains(slug),
    queryFn: async () => (await api.listSendingDomains(slug)).data.domains,
  })
  const verifiedDomains = (domainsQuery.data ?? []).filter(
    (d) => d.status === "verified",
  )
  const selectedDomainVerified = verifiedDomains.some((d) => d.id === domainId)

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 200, offset: 0 })).data.lists,
  })

  async function invalidate() {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.campaign(slug, campaign.id),
    })
    await queryClient.invalidateQueries({
      queryKey: queryKeys.campaigns(slug),
    })
  }

  const save = useMutation({
    mutationFn: (v: {
      name: string
      subject: string
      bodyHtml: string
      bodyText: string
      fromName: string
      fromLocalPart: string
    }) =>
      api.updateCampaign(slug, campaign.id, {
        name: v.name.trim(),
        subject: v.subject,
        body_html: v.bodyHtml,
        body_text: v.bodyText,
        from_name: v.fromName,
        from_local_part: v.fromLocalPart,
        sending_domain_id: domainId || undefined,
        list_ids: listIds,
        segments: campaign.segments ?? undefined,
      }),
    onSuccess: async () => {
      await invalidate()
      toast.success("Campaign saved.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const start = useMutation({
    mutationFn: () => api.startCampaign(slug, campaign.id),
    onSuccess: async () => {
      await invalidate()
      toast.success("Campaign started.")
      setConfirmStart(false)
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      setConfirmStart(false)
    },
  })

  const lifecycle = useMutation({
    mutationFn: (action: "pause" | "resume" | "cancel") => {
      if (action === "pause") return api.pauseCampaign(slug, campaign.id)
      if (action === "resume") return api.resumeCampaign(slug, campaign.id)
      return api.cancelCampaign(slug, campaign.id)
    },
    onSuccess: async () => {
      await invalidate()
      setConfirmCancel(false)
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      setConfirmCancel(false)
    },
  })

  const form = useForm({
    defaultValues: {
      name: campaign.name,
      subject: campaign.subject,
      bodyHtml: campaign.body_html,
      bodyText: campaign.body_text,
      fromName: campaign.from_name,
      fromLocalPart: campaign.from_local_part,
    },
    onSubmit: async ({ value }) => {
      await save.mutateAsync(value).catch(() => {})
    },
  })

  const canStart = isDraft && selectedDomainVerified && !save.isPending

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">{campaign.name}</h1>
          <CampaignStatusBadge status={campaign.status} />
        </div>
        {canManage && (
          <div className="flex gap-2">
            {campaign.status === "running" && (
              <Button
                variant="outline"
                disabled={lifecycle.isPending}
                onClick={() => lifecycle.mutate("pause")}
              >
                Pause
              </Button>
            )}
            {campaign.status === "paused" && (
              <Button
                variant="outline"
                disabled={lifecycle.isPending}
                onClick={() => lifecycle.mutate("resume")}
              >
                Resume
              </Button>
            )}
            {(campaign.status === "running" ||
              campaign.status === "paused") && (
              <Button
                variant="destructive"
                disabled={lifecycle.isPending}
                onClick={() => setConfirmCancel(true)}
              >
                Cancel campaign
              </Button>
            )}
            {isDraft && (
              <Button
                disabled={!canStart}
                onClick={() => setConfirmStart(true)}
              >
                Start campaign
              </Button>
            )}
          </div>
        )}
      </header>

      {isDraft && !selectedDomainVerified && (
        <Alert>
          <AlertTitle>A verified sending domain is required</AlertTitle>
          <AlertDescription>
            Select a verified sending domain below before this campaign can be
            started.
          </AlertDescription>
        </Alert>
      )}

      {isAutoPaused(campaign) && (
        <Alert variant="destructive">
          <AlertTitle>Campaign auto-paused</AlertTitle>
          <AlertDescription>
            Sending was paused automatically after reaching{" "}
            {campaign.max_send_errors} send errors. Review and resume, or
            cancel the campaign.
          </AlertDescription>
        </Alert>
      )}

      {campaign.status !== "draft" && (
        <SendProgress campaign={campaign} />
      )}

      {isDraft ? (
        <form
          className="flex flex-col gap-6"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            form.handleSubmit()
          }}
        >
          <Card>
            <CardHeader>
              <CardTitle>Content</CardTitle>
              <CardDescription>
                Edit the campaign’s subject and content. HTML is sent as-is.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
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
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    error={fieldError(field.state.meta.errors)}
                  />
                )}
              </form.Field>
              <form.Field
                name="subject"
                validators={{
                  onBlur: compose(rules.required("Enter a subject.")),
                }}
              >
                {(field) => (
                  <FormField
                    label="Subject"
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
                  <FormField label="HTML body">
                    <Textarea
                      rows={8}
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </FormField>
                )}
              </form.Field>
              <form.Field name="bodyText">
                {(field) => (
                  <FormField label="Plain-text body">
                    <Textarea
                      rows={5}
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </FormField>
                )}
              </form.Field>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Sender</CardTitle>
              <CardDescription>
                Choose a verified sending domain and the from address.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="flex flex-col gap-1.5">
                <Label>Sending domain</Label>
                <Select
                  value={domainId || "none"}
                  onValueChange={(v) =>
                    setDomainId(v === "none" ? "" : v)
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select a verified domain" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No domain selected</SelectItem>
                    {verifiedDomains.map((d) => (
                      <SelectItem key={d.id} value={d.id}>
                        {d.domain}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {verifiedDomains.length === 0 && (
                  <p className="text-xs text-muted-foreground">
                    No verified domains yet. Verify a domain under Sending
                    Domains first.
                  </p>
                )}
              </div>
              <div className="grid grid-cols-2 gap-4">
                <form.Field name="fromName">
                  {(field) => (
                    <FormField
                      label="From name"
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  )}
                </form.Field>
                <form.Field name="fromLocalPart">
                  {(field) => (
                    <FormField
                      label="From address (local part)"
                      placeholder="news"
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  )}
                </form.Field>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Recipients</CardTitle>
              <CardDescription>
                Select one or more lists to send to.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <AsyncState
                query={listsQuery}
                isEmpty={(d) => d.length === 0}
                emptyTitle="No lists"
                emptyMessage="Create a list of subscribers to target."
              >
                {(lists) => (
                  <div className="flex flex-col gap-2">
                    {lists.map((list) => (
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
                )}
              </AsyncState>
            </CardContent>
          </Card>

          {canManage && (
            <div>
              <Button type="submit" disabled={save.isPending}>
                {save.isPending ? "Saving…" : "Save changes"}
              </Button>
            </div>
          )}
        </form>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Content</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <p>
              <span className="text-muted-foreground">Subject: </span>
              {campaign.subject}
            </p>
            <p>
              <span className="text-muted-foreground">From: </span>
              {campaign.from_name} &lt;{campaign.from_local_part}&gt;
            </p>
          </CardContent>
        </Card>
      )}

      <ConfirmDialog
        open={confirmStart}
        onOpenChange={setConfirmStart}
        title="Start this campaign?"
        description="Sending begins immediately and cannot be undone. You can pause or cancel it once running."
        confirmLabel="Start sending"
        busy={start.isPending}
        onConfirm={() => start.mutate()}
      />

      <ConfirmDialog
        open={confirmCancel}
        onOpenChange={setConfirmCancel}
        title="Cancel this campaign?"
        description="The campaign will stop sending and cannot be resumed."
        confirmLabel="Cancel campaign"
        busy={lifecycle.isPending}
        onConfirm={() => lifecycle.mutate("cancel")}
      />
    </div>
  )
}

function SendProgress({ campaign }: { campaign: CampaignView }) {
  const { sent, failed, remaining, total } = campaignProgress(campaign)
  const done = total > 0 ? Math.round(((sent + failed) / total) * 100) : 0
  return (
    <Card>
      <CardHeader>
        <CardTitle>Send progress</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <Progress value={done} />
        <div className="grid grid-cols-3 gap-4 text-sm">
          <div>
            <p className="text-2xl font-semibold">{sent}</p>
            <p className="text-muted-foreground">Sent</p>
          </div>
          <div>
            <p className="text-2xl font-semibold">{failed}</p>
            <p className="text-muted-foreground">Failed</p>
          </div>
          <div>
            <p className="text-2xl font-semibold">{remaining}</p>
            <p className="text-muted-foreground">Remaining</p>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

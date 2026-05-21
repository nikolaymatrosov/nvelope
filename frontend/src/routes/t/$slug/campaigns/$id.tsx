import { Link, createFileRoute } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ImageIcon } from "lucide-react"
import { CampaignStatusBadge } from "./index"
import type {
  CampaignView,
  MediaAssetView,
  VisualDoc,
} from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { ApiError, errorMessage } from "@/lib/errors"
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
import { VisualEmailEditor } from "@/components/visual-editor/VisualEmailEditor"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
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
import { MediaPicker } from "@/components/common/media-picker"
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
          <>
            {campaign.status !== "draft" && (
              <div>
                <Link
                  to="/t/$slug/campaigns/$id/analytics"
                  params={{ slug, id: campaign.id }}
                  className="text-sm font-medium text-primary hover:underline"
                >
                  View analytics →
                </Link>
              </div>
            )}
            <CampaignEditor
              key={campaign.id}
              slug={slug}
              campaign={campaign}
              canManage={canManage}
            />
          </>
        )}
      </AsyncState>
    </div>
  )
}

// Decide which editor surface to show for a campaign on first render.
// A row with a non-null `body_doc` came back from the visual save path;
// a fresh campaign (no doc, no html) lands in visual mode so the operator
// can author from a blank canvas. Pre-Phase-7 raw-HTML campaigns
// (`body_doc == null` AND `body_html` non-empty) stay in code-only mode
// — they only switch on the explicit US4 "convert to visual" affordance.
function initialEditorMode(campaign: CampaignView): "visual" | "code" {
  if (campaign.body_doc) return "visual"
  if (campaign.body_html.trim() === "") return "visual"
  return "code"
}

const EMPTY_VISUAL_DOC: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [] }],
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
  const [sendBlock, setSendBlock] = useState<"quota" | "suspended" | null>(
    null,
  )
  const [mediaPickerOpen, setMediaPickerOpen] = useState(false)
  const bodyHtmlRef = useRef<HTMLTextAreaElement | null>(null)
  const { can } = usePermissions(slug)
  const canPickMedia = can("media:get")

  // Phase 7 — visual editor surface. Mode is mutable so the operator can
  // convert a legacy raw-HTML campaign to visual (T092) or opt out of the
  // visual editor on a visual row (T093 / FR-029).
  const [editorMode, setEditorMode] = useState<"visual" | "code">(() =>
    initialEditorMode(campaign),
  )
  const [confirmOptOut, setConfirmOptOut] = useState(false)
  const [bodyDoc, setBodyDoc] = useState<VisualDoc>(
    () => campaign.body_doc ?? EMPTY_VISUAL_DOC,
  )
  // Optimistic-concurrency token (FR-009). Updated every time the row's
  // visual save returns a new `updatedAt`; on `409 stale_row` the operator
  // can Reload (refetch + discard local edits) or Force overwrite (refetch
  // + retry the save with the row's now-current timestamp).
  const [ifUnmodifiedSince, setIfUnmodifiedSince] = useState<string>(
    campaign.updated_at,
  )

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

  async function refetchCampaign(): Promise<CampaignView | undefined> {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.campaign(slug, campaign.id),
    })
    const fresh = (await api.getCampaign(slug, campaign.id)).data
    setBodyDoc(fresh.body_doc ?? EMPTY_VISUAL_DOC)
    setIfUnmodifiedSince(fresh.updated_at)
    return fresh
  }

  // Visual save (Phase 7). Carries the operator's structured doc, the
  // current subject, the optional theme override, and the row's last-known
  // `updated_at` as the FR-009 concurrency token. On `409 stale_row` the
  // sonner toast surfaces both Reload and Force-overwrite affordances.
  const saveVisual = useMutation({
    mutationFn: (v: { subject: string }) =>
      api.campaigns.saveVisual(slug, campaign.id, {
        subject: v.subject,
        bodyDoc,
        theme: campaign.theme ?? null,
        ifUnmodifiedSince,
      }),
    onSuccess: async (res) => {
      setIfUnmodifiedSince(res.data.updatedAt)
      await invalidate()
      const warnings = res.data.warnings.length
      if (warnings > 0) {
        toast.warning(
          `Campaign saved with ${warnings} content warning${warnings === 1 ? "" : "s"}.`,
        )
      } else {
        toast.success("Campaign saved.")
      }
    },
    onError: (e, vars) => {
      if (e instanceof ApiError && e.status === 409 && e.slug === "stale_row") {
        const currentUpdatedAt =
          typeof e.data?.currentUpdatedAt === "string"
            ? e.data.currentUpdatedAt
            : null
        toast.warning("Changed in another tab/session", {
          duration: 12_000,
          action: {
            label: "Reload",
            onClick: () => {
              void refetchCampaign()
            },
          },
          // sonner's secondary `cancel` slot is used for the second
          // affordance. The operator's pending body stays in local state
          // so Force-overwrite re-issues the same save with the fresh
          // token.
          cancel: {
            label: "Force overwrite",
            onClick: () => {
              if (!currentUpdatedAt) {
                void refetchCampaign().then((fresh) => {
                  if (!fresh) return
                  saveVisual.mutate(vars)
                })
                return
              }
              setIfUnmodifiedSince(currentUpdatedAt)
              // Re-issue with the new token. We bypass the React state
              // update timing by passing the token directly through a
              // throwaway mutation call.
              api.campaigns
                .saveVisual(slug, campaign.id, {
                  subject: vars.subject,
                  bodyDoc,
                  theme: campaign.theme ?? null,
                  ifUnmodifiedSince: currentUpdatedAt,
                })
                .then(async (res) => {
                  setIfUnmodifiedSince(res.data.updatedAt)
                  await invalidate()
                  toast.success("Campaign saved.")
                })
                .catch((err) => toast.error(errorMessage(err)))
            },
          },
        })
        return
      }
      toast.error(errorMessage(e))
    },
  })

  // Convert legacy raw-HTML → VisualDoc (T092). Non-persisting: the
  // returned doc lands in local state and the editor swaps into visual
  // mode; the operator reviews any rawhtml-fallback warnings and saves
  // through the regular visual PUT.
  const convertToVisual = useMutation({
    mutationFn: () => api.campaigns.convertToVisual(slug, campaign.id),
    onSuccess: (res) => {
      setBodyDoc(res.data.bodyDoc)
      setEditorMode("visual")
      const warnings = res.data.warnings.length
      if (warnings > 0) {
        toast.warning(
          `Converted to visual editor with ${warnings} block${warnings === 1 ? "" : "s"} preserved as raw HTML. Review and save to keep the visual document.`,
          { duration: 12_000 },
        )
      } else {
        toast.success("Converted to visual editor. Review and save to keep it.")
      }
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  // Opt out of the visual editor (T093 / FR-029). Persists immediately —
  // clears body_doc + theme so the row reverts to a code-only campaign
  // while body_html / body_text stay intact.
  const optOutVisual = useMutation({
    mutationFn: () => api.campaigns.optOutVisual(slug, campaign.id),
    onSuccess: async () => {
      setEditorMode("code")
      setConfirmOptOut(false)
      await invalidate()
      toast.success("Switched to HTML-only mode. The structured document was cleared.")
    },
    onError: (e) => {
      setConfirmOptOut(false)
      toast.error(errorMessage(e))
    },
  })

  const start = useMutation({
    mutationFn: () => api.startCampaign(slug, campaign.id),
    onSuccess: async () => {
      await invalidate()
      toast.success("Campaign started.")
      setConfirmStart(false)
      setSendBlock(null)
    },
    onError: (e) => {
      setConfirmStart(false)
      if (e instanceof ApiError && e.slug === "quota_exceeded") {
        setSendBlock("quota")
        return
      }
      if (e instanceof ApiError && e.slug === "tenant_suspended") {
        setSendBlock("suspended")
        return
      }
      toast.error(errorMessage(e))
    },
  })

  const archive = useMutation({
    mutationFn: (visible: boolean) =>
      api.setCampaignArchive(slug, campaign.id, visible),
    onSuccess: async (_, visible) => {
      await invalidate()
      toast.success(
        visible
          ? "Campaign is now visible in the public archive."
          : "Campaign is hidden from the public archive.",
      )
    },
    onError: (e) => toast.error(errorMessage(e)),
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
      if (editorMode === "visual") {
        // The legacy update handles name + sender + recipients (the
        // visual save endpoint only accepts subject + bodyDoc + theme).
        // Run both so a single click persists the full draft.
        await save
          .mutateAsync({
            ...value,
            bodyHtml: campaign.body_html,
            bodyText: campaign.body_text,
          })
          .catch(() => {})
        await saveVisual.mutateAsync({ subject: value.subject }).catch(() => {})
        return
      }
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

      {sendBlock === "quota" && (
        <Alert variant="destructive" data-testid="campaign-quota-blocked">
          <AlertTitle>Send allowance reached</AlertTitle>
          <AlertDescription>
            This campaign was not started because the workspace has used its
            plan's send allowance for the current period. Review your{" "}
            <Link
              to="/t/$slug/billing/usage"
              params={{ slug }}
              className="font-medium underline underline-offset-2"
            >
              usage and plan
            </Link>{" "}
            to continue sending.
          </AlertDescription>
        </Alert>
      )}

      {sendBlock === "suspended" && (
        <Alert variant="destructive" data-testid="campaign-suspended-blocked">
          <AlertTitle>Account suspended</AlertTitle>
          <AlertDescription>
            This campaign was not started because the workspace is suspended
            for non-payment. Settle the outstanding balance in{" "}
            <Link
              to="/t/$slug/billing"
              params={{ slug }}
              className="font-medium underline underline-offset-2"
            >
              billing
            </Link>{" "}
            to re-enable sending.
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

      {campaign.status !== "draft" && canManage && (
        <Card data-testid="archive-visibility-card">
          <CardHeader>
            <CardTitle>Public archive</CardTitle>
            <CardDescription>
              When enabled, this campaign appears in the tenant's public
              archive index, on its standalone page, and in the RSS feed.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={Boolean(campaign.archive_visible)}
                disabled={archive.isPending}
                onCheckedChange={(c) => archive.mutate(Boolean(c))}
                data-testid="archive-visible-toggle"
              />
              Visible in the public archive
            </label>
          </CardContent>
        </Card>
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
              {editorMode === "visual" && canManage ? (
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="campaign-visual-editor">Body</Label>
                  <VisualEmailEditor
                    slug={slug}
                    value={bodyDoc}
                    onChange={setBodyDoc}
                    onOptOutVisual={
                      campaign.body_doc
                        ? () => setConfirmOptOut(true)
                        : undefined
                    }
                  />
                </div>
              ) : (
                <>
                  <form.Field name="bodyHtml">
                    {(field) => (
                      <div className="flex flex-col gap-1.5">
                        <div className="flex items-center justify-between">
                          <Label htmlFor="campaign-body-html">HTML body</Label>
                          <div className="flex items-center gap-2">
                            {canManage && field.state.value.trim() !== "" && !campaign.body_doc && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                disabled={convertToVisual.isPending}
                                onClick={() => convertToVisual.mutate()}
                                data-testid="convert-to-visual"
                              >
                                {convertToVisual.isPending
                                  ? "Converting…"
                                  : "Convert to visual editor"}
                              </Button>
                            )}
                            {canPickMedia && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() => setMediaPickerOpen(true)}
                                data-testid="open-media-picker"
                              >
                                <ImageIcon /> Insert from media library
                              </Button>
                            )}
                          </div>
                        </div>
                        <Textarea
                          id="campaign-body-html"
                          ref={bodyHtmlRef}
                          rows={8}
                          value={field.state.value}
                          onChange={(e) => field.handleChange(e.target.value)}
                        />
                        <MediaPicker
                          slug={slug}
                          open={mediaPickerOpen}
                          onOpenChange={setMediaPickerOpen}
                          onPick={(asset: MediaAssetView) => {
                            const ta = bodyHtmlRef.current
                            const value = field.state.value
                            const insert = asset.public_url
                            if (ta) {
                              const start = ta.selectionStart
                              const end = ta.selectionEnd
                              const next =
                                value.slice(0, start) +
                                insert +
                                value.slice(end)
                              field.handleChange(next)
                              // Restore caret after the inserted URL.
                              requestAnimationFrame(() => {
                                const pos = start + insert.length
                                ta.focus()
                                ta.setSelectionRange(pos, pos)
                              })
                            } else {
                              field.handleChange(value + insert)
                            }
                          }}
                        />
                      </div>
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
                </>
              )}
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
              <Button
                type="submit"
                disabled={save.isPending || saveVisual.isPending}
              >
                {save.isPending || saveVisual.isPending
                  ? "Saving…"
                  : "Save changes"}
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

      <Dialog open={confirmOptOut} onOpenChange={setConfirmOptOut}>
        <DialogContent data-testid="opt-out-visual-dialog">
          <DialogHeader>
            <DialogTitle>Switch to HTML-only editing?</DialogTitle>
            <DialogDescription>
              Your structured visual document will be discarded. The last
              saved HTML body stays intact so the campaign remains sendable,
              but blocks, columns, and merge-tag chips will no longer be
              available unless you convert back later.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setConfirmOptOut(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={optOutVisual.isPending}
              onClick={() => optOutVisual.mutate()}
              data-testid="opt-out-visual-confirm"
            >
              {optOutVisual.isPending ? "Switching…" : "Switch to HTML only"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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

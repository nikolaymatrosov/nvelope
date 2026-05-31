import { Link, createFileRoute } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ImageIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { CampaignStatusBadge } from "./index"
import type {
  CampaignView,
  MediaAssetView,
  Theme,
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
import { ThreePaneEditor } from "@/components/visual-editor/ThreePaneEditor"
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
  const { t } = useTranslation("campaigns")
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
          {t("detail.backToCampaigns")}
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
                  {t("detail.viewAnalytics")}
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
  const { t } = useTranslation(["campaigns", "common"])
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
  // Theme override for the row. null = inherit tenant branding; an object =
  // pinned override. The ThemeControls panel inside VisualEmailEditor mutates
  // this via `setThemeOverride`; it is forwarded to the visual save body
  // (per FR-022 / FR-023 / FR-024 — T109).
  const [themeOverride, setThemeOverride] = useState<Theme | null>(
    () => campaign.theme ?? null,
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
      toast.success(t("detail.saveSuccess"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  async function refetchCampaign(): Promise<CampaignView | undefined> {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.campaign(slug, campaign.id),
    })
    const fresh = (await api.getCampaign(slug, campaign.id)).data
    setBodyDoc(fresh.body_doc ?? EMPTY_VISUAL_DOC)
    setThemeOverride(fresh.theme ?? null)
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
        theme: themeOverride,
        ifUnmodifiedSince,
      }),
    onSuccess: async (res) => {
      setIfUnmodifiedSince(res.data.updatedAt)
      await invalidate()
      const warnings = res.data.warnings.length
      if (warnings > 0) {
        toast.warning(t("detail.saveWarnings", { count: warnings }))
      } else {
        toast.success(t("detail.saveSuccess"))
      }
    },
    onError: (e, vars) => {
      if (e instanceof ApiError && e.status === 409 && e.slug === "stale_row") {
        const currentUpdatedAt =
          typeof e.data?.currentUpdatedAt === "string"
            ? e.data.currentUpdatedAt
            : null
        toast.warning(t("visual.staleTitle"), {
          duration: 12_000,
          action: {
            label: t("visual.reload"),
            onClick: () => {
              void refetchCampaign()
            },
          },
          // sonner's secondary `cancel` slot is used for the second
          // affordance. The operator's pending body stays in local state
          // so Force-overwrite re-issues the same save with the fresh
          // token.
          cancel: {
            label: t("visual.forceOverwrite"),
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
                  theme: themeOverride,
                  ifUnmodifiedSince: currentUpdatedAt,
                })
                .then(async (res) => {
                  setIfUnmodifiedSince(res.data.updatedAt)
                  await invalidate()
                  toast.success(t("detail.saveSuccess"))
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
        toast.warning(t("visual.convertedWarnings", { count: warnings }), {
          duration: 12_000,
        })
      } else {
        toast.success(t("visual.convertedSuccess"))
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
      toast.success(t("visual.optOutSuccess"))
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
      toast.success(t("detail.startSuccess"))
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
          ? t("detail.archive.shown")
          : t("detail.archive.hidden"),
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
                {t("detail.actions.pause")}
              </Button>
            )}
            {campaign.status === "paused" && (
              <Button
                variant="outline"
                disabled={lifecycle.isPending}
                onClick={() => lifecycle.mutate("resume")}
              >
                {t("detail.actions.resume")}
              </Button>
            )}
            {(campaign.status === "running" ||
              campaign.status === "paused") && (
              <Button
                variant="destructive"
                disabled={lifecycle.isPending}
                onClick={() => setConfirmCancel(true)}
              >
                {t("detail.actions.cancel")}
              </Button>
            )}
            {isDraft && (
              <Button
                disabled={!canStart}
                onClick={() => setConfirmStart(true)}
              >
                {t("detail.actions.start")}
              </Button>
            )}
          </div>
        )}
      </header>

      {isDraft && !selectedDomainVerified && (
        <Alert>
          <AlertTitle>{t("detail.domainRequired.title")}</AlertTitle>
          <AlertDescription>
            {t("detail.domainRequired.description")}
          </AlertDescription>
        </Alert>
      )}

      {sendBlock === "quota" && (
        <Alert variant="destructive" data-testid="campaign-quota-blocked">
          <AlertTitle>{t("detail.quotaBlocked.title")}</AlertTitle>
          <AlertDescription>
            {t("detail.quotaBlocked.descriptionBefore")}
            <Link
              to="/t/$slug/billing/usage"
              params={{ slug }}
              className="font-medium underline underline-offset-2"
            >
              {t("detail.quotaBlocked.link")}
            </Link>
            {t("detail.quotaBlocked.descriptionAfter")}
          </AlertDescription>
        </Alert>
      )}

      {sendBlock === "suspended" && (
        <Alert variant="destructive" data-testid="campaign-suspended-blocked">
          <AlertTitle>{t("detail.suspendedBlocked.title")}</AlertTitle>
          <AlertDescription>
            {t("detail.suspendedBlocked.descriptionBefore")}
            <Link
              to="/t/$slug/billing"
              params={{ slug }}
              className="font-medium underline underline-offset-2"
            >
              {t("detail.suspendedBlocked.link")}
            </Link>
            {t("detail.suspendedBlocked.descriptionAfter")}
          </AlertDescription>
        </Alert>
      )}

      {isAutoPaused(campaign) && (
        <Alert variant="destructive">
          <AlertTitle>{t("detail.autoPaused.title")}</AlertTitle>
          <AlertDescription>
            {t("detail.autoPaused.description", {
              count: campaign.max_send_errors,
            })}
          </AlertDescription>
        </Alert>
      )}

      {campaign.status !== "draft" && (
        <SendProgress campaign={campaign} />
      )}

      {campaign.status !== "draft" && canManage && (
        <Card data-testid="archive-visibility-card">
          <CardHeader>
            <CardTitle>{t("detail.archive.title")}</CardTitle>
            <CardDescription>
              {t("detail.archive.description")}
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
              {t("detail.archive.toggle")}
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
              <CardTitle>{t("detail.content.title")}</CardTitle>
              <CardDescription>
                {t("detail.content.description")}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <form.Field
                name="name"
                validators={{
                  onBlur: compose(
                    rules.required(t("detail.content.nameRequired")),
                  ),
                }}
              >
                {(field) => (
                  <FormField
                    label={t("detail.content.nameLabel")}
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
                  onBlur: compose(
                    rules.required(t("detail.content.subjectRequired")),
                  ),
                }}
              >
                {(field) => (
                  <FormField
                    label={t("detail.content.subjectLabel")}
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
                  <Label htmlFor="campaign-visual-editor">
                    {t("detail.content.bodyLabel")}
                  </Label>
                  <ThreePaneEditor
                    slug={slug}
                    value={bodyDoc}
                    onChange={setBodyDoc}
                    theme={themeOverride}
                    onThemeChange={setThemeOverride}
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
                          <Label htmlFor="campaign-body-html">
                            {t("detail.content.htmlBodyLabel")}
                          </Label>
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
                                  ? t("visual.converting")
                                  : t("visual.convert")}
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
                                <ImageIcon />{" "}
                                {t("detail.content.insertFromMedia")}
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
                              const selStart = ta.selectionStart
                              const selEnd = ta.selectionEnd
                              const next =
                                value.slice(0, selStart) +
                                insert +
                                value.slice(selEnd)
                              field.handleChange(next)
                              // Restore caret after the inserted URL.
                              requestAnimationFrame(() => {
                                const pos = selStart + insert.length
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
                      <FormField label={t("detail.content.textBodyLabel")}>
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
              <CardTitle>{t("detail.sender.title")}</CardTitle>
              <CardDescription>
                {t("detail.sender.description")}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="flex flex-col gap-1.5">
                <Label>{t("detail.sender.domainLabel")}</Label>
                <Select
                  value={domainId || "none"}
                  onValueChange={(v) =>
                    setDomainId(v === "none" ? "" : v)
                  }
                >
                  <SelectTrigger>
                    <SelectValue
                      placeholder={t("detail.sender.domainPlaceholder")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">
                      {t("detail.sender.noDomain")}
                    </SelectItem>
                    {verifiedDomains.map((d) => (
                      <SelectItem key={d.id} value={d.id}>
                        {d.domain}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {verifiedDomains.length === 0 && (
                  <p className="text-xs text-muted-foreground">
                    {t("detail.sender.noVerifiedDomains")}
                  </p>
                )}
              </div>
              <div className="grid grid-cols-2 gap-4">
                <form.Field name="fromName">
                  {(field) => (
                    <FormField
                      label={t("detail.sender.fromNameLabel")}
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  )}
                </form.Field>
                <form.Field name="fromLocalPart">
                  {(field) => (
                    <FormField
                      label={t("detail.sender.fromLocalPartLabel")}
                      placeholder={t("detail.sender.fromLocalPartPlaceholder")}
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
              <CardTitle>{t("detail.recipients.title")}</CardTitle>
              <CardDescription>
                {t("detail.recipients.description")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <AsyncState
                query={listsQuery}
                isEmpty={(d) => d.length === 0}
                emptyTitle={t("detail.recipients.emptyTitle")}
                emptyMessage={t("detail.recipients.emptyMessage")}
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
                  ? t("detail.saving")
                  : t("detail.save")}
              </Button>
            </div>
          )}
        </form>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>{t("detail.content.title")}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <p>
              <span className="text-muted-foreground">
                {t("detail.summary.subjectLabel")}
              </span>
              {campaign.subject}
            </p>
            <p>
              <span className="text-muted-foreground">
                {t("detail.summary.fromLabel")}
              </span>
              {campaign.from_name} &lt;{campaign.from_local_part}&gt;
            </p>
          </CardContent>
        </Card>
      )}

      <ConfirmDialog
        open={confirmStart}
        onOpenChange={setConfirmStart}
        title={t("detail.confirmStart.title")}
        description={t("detail.confirmStart.description")}
        confirmLabel={t("detail.confirmStart.confirm")}
        busy={start.isPending}
        onConfirm={() => start.mutate()}
      />

      <ConfirmDialog
        open={confirmCancel}
        onOpenChange={setConfirmCancel}
        title={t("detail.confirmCancel.title")}
        description={t("detail.confirmCancel.description")}
        confirmLabel={t("detail.confirmCancel.confirm")}
        busy={lifecycle.isPending}
        onConfirm={() => lifecycle.mutate("cancel")}
      />

      <Dialog open={confirmOptOut} onOpenChange={setConfirmOptOut}>
        <DialogContent data-testid="opt-out-visual-dialog">
          <DialogHeader>
            <DialogTitle>{t("visual.optOut.title")}</DialogTitle>
            <DialogDescription>
              {t("visual.optOut.description")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setConfirmOptOut(false)}
            >
              {t("common:actions.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={optOutVisual.isPending}
              onClick={() => optOutVisual.mutate()}
              data-testid="opt-out-visual-confirm"
            >
              {optOutVisual.isPending
                ? t("visual.optOut.switching")
                : t("visual.optOut.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function SendProgress({ campaign }: { campaign: CampaignView }) {
  const { t } = useTranslation("campaigns")
  const { sent, failed, remaining, total } = campaignProgress(campaign)
  const done = total > 0 ? Math.round(((sent + failed) / total) * 100) : 0
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("sendProgress.title")}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <Progress value={done} />
        <div className="grid grid-cols-3 gap-4 text-sm">
          <div>
            <p className="text-2xl font-semibold">{sent}</p>
            <p className="text-muted-foreground">{t("sendProgress.sent")}</p>
          </div>
          <div>
            <p className="text-2xl font-semibold">{failed}</p>
            <p className="text-muted-foreground">
              {t("sendProgress.failed")}
            </p>
          </div>
          <div>
            <p className="text-2xl font-semibold">{remaining}</p>
            <p className="text-muted-foreground">
              {t("sendProgress.remaining")}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

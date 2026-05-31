import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { TemplateView, Theme, VisualDoc } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { ApiError, errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ThreePaneEditor } from "@/components/visual-editor/ThreePaneEditor"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/templates/$id")({
  component: TemplateDetail,
})

export function TemplateDetail() {
  const { slug, id } = Route.useParams()
  const { t } = useTranslation("templates")
  const { can } = usePermissions(slug)
  const canManage = can("campaigns:manage")

  const query = useQuery({
    queryKey: queryKeys.template(slug, id),
    queryFn: async () => (await api.getTemplate(slug, id)).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/templates"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          {t("detail.backToTemplates")}
        </Link>
      </div>

      <AsyncState query={query}>
        {(template) => (
          <EditTemplateCard
            key={template.id}
            slug={slug}
            template={template}
            canManage={canManage}
          />
        )}
      </AsyncState>
    </div>
  )
}

// Decide which editor surface to show for a template on first render.
// Mirrors the campaign editor's `initialEditorMode`: a row with a non-null
// `body_doc` came back from the visual save path; a fresh template lands
// in visual mode for campaign-kind templates so the operator can author
// from a blank canvas. Pre-Phase-7 raw-HTML templates (body_doc == null
// AND body_html non-empty) stay in code-only mode — and transactional
// templates never enter the visual editor (per plan.md / spec.md scope).
function initialTemplateEditorMode(template: TemplateView): "visual" | "code" {
  if (template.kind === "transactional") return "code"
  if (template.body_doc) return "visual"
  if (template.body_html.trim() === "") return "visual"
  return "code"
}

const EMPTY_VISUAL_DOC: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [] }],
}

function EditTemplateCard({
  slug,
  template,
  canManage,
}: {
  slug: string
  template: TemplateView
  canManage: boolean
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["templates", "common"])

  // Mode is mutable so the operator can convert a legacy raw-HTML
  // template to visual (T092) or opt out of the visual editor on a
  // visual row (T093 / FR-029). Transactional templates never enter
  // visual mode — the convert button is gated on kind === "campaign".
  const [editorMode, setEditorMode] = useState<"visual" | "code">(() =>
    initialTemplateEditorMode(template),
  )
  const [confirmOptOut, setConfirmOptOut] = useState(false)
  const [bodyDoc, setBodyDoc] = useState<VisualDoc>(
    () => template.body_doc ?? EMPTY_VISUAL_DOC,
  )
  // Theme override for the template. null = inherit tenant branding; an
  // object = pinned override (per FR-022 / FR-023 / FR-024 — T109).
  const [themeOverride, setThemeOverride] = useState<Theme | null>(
    () => template.theme ?? null,
  )
  // Optimistic-concurrency token (FR-009). Updated every time the row's
  // visual save returns a new `updatedAt`; on 409 stale_row the operator
  // can Reload (refetch + discard local edits) or Force overwrite (refetch
  // + retry the save with the row's now-current timestamp).
  const [ifUnmodifiedSince, setIfUnmodifiedSince] = useState<string>(
    template.updated_at,
  )

  async function invalidate() {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.templates(slug),
    })
    await queryClient.invalidateQueries({
      queryKey: queryKeys.template(slug, template.id),
    })
  }

  const save = useMutation({
    mutationFn: (v: {
      name: string
      subject: string
      bodyHtml: string
      bodyText: string
    }) =>
      api.updateTemplate(slug, template.id, {
        name: v.name.trim(),
        subject: v.subject,
        body_html: v.bodyHtml,
        body_text: v.bodyText,
      }),
    onSuccess: async () => {
      await invalidate()
      toast.success(t("detail.saveSuccess"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  // Convert legacy raw-HTML → VisualDoc (T092). Non-persisting: the
  // returned doc lands in local state and the editor swaps into visual
  // mode; the operator reviews any rawhtml-fallback warnings and saves
  // through the regular visual PUT.
  const convertToVisual = useMutation({
    mutationFn: () => api.templates.convertToVisual(slug, template.id),
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
  // clears body_doc + theme so the row reverts to a code-only template.
  const optOutVisual = useMutation({
    mutationFn: () => api.templates.optOutVisual(slug, template.id),
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

  async function refetchTemplate(): Promise<TemplateView | undefined> {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.template(slug, template.id),
    })
    const fresh = (await api.getTemplate(slug, template.id)).data
    setBodyDoc(fresh.body_doc ?? EMPTY_VISUAL_DOC)
    setThemeOverride(fresh.theme ?? null)
    setIfUnmodifiedSince(fresh.updated_at)
    return fresh
  }

  // Visual save (Phase 7 / T080). Mirrors the campaign editor's saveVisual
  // mutation: structured doc + subject + the FR-009 concurrency token.
  // The browser-facing body also carries name + kind because Go's
  // PUT /templates/{id}/visual contract takes them (immutable post-create,
  // but forwarded for completeness).
  const saveVisual = useMutation({
    mutationFn: (v: { name: string; subject: string }) =>
      api.templates.saveVisual(slug, template.id, {
        name: v.name.trim(),
        kind: template.kind,
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
              void refetchTemplate()
            },
          },
          cancel: {
            label: t("visual.forceOverwrite"),
            onClick: () => {
              if (!currentUpdatedAt) {
                void refetchTemplate().then((fresh) => {
                  if (!fresh) return
                  saveVisual.mutate(vars)
                })
                return
              }
              setIfUnmodifiedSince(currentUpdatedAt)
              api.templates
                .saveVisual(slug, template.id, {
                  name: vars.name.trim(),
                  kind: template.kind,
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

  const form = useForm({
    defaultValues: {
      name: template.name,
      subject: template.subject,
      bodyHtml: template.body_html,
      bodyText: template.body_text,
    },
    onSubmit: async ({ value }) => {
      if (editorMode === "visual") {
        // The legacy update handles name (the visual save endpoint only
        // accepts subject + bodyDoc + theme). Run both so a single click
        // persists the full template.
        await save
          .mutateAsync({
            ...value,
            bodyHtml: template.body_html,
            bodyText: template.body_text,
          })
          .catch(() => {})
        await saveVisual
          .mutateAsync({ name: value.name, subject: value.subject })
          .catch(() => {})
        return
      }
      await save.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {template.name}
          <Badge variant="secondary">{t(`kind.${template.kind}`)}</Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
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
              onBlur: compose(rules.required(t("detail.nameRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("detail.nameLabel")}
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
              onBlur: compose(rules.required(t("detail.subjectRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("detail.subjectLabel")}
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
              <Label htmlFor="template-visual-editor">
                {t("detail.bodyLabel")}
              </Label>
              <ThreePaneEditor
                slug={slug}
                value={bodyDoc}
                onChange={setBodyDoc}
                theme={themeOverride}
                onThemeChange={setThemeOverride}
                onOptOutVisual={
                  template.body_doc
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
                      <Label>{t("detail.htmlBodyLabel")}</Label>
                      {canManage &&
                        template.kind === "campaign" &&
                        field.state.value.trim() !== "" &&
                        !template.body_doc && (
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
                    </div>
                    <Textarea
                      rows={8}
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </div>
                )}
              </form.Field>
              <form.Field name="bodyText">
                {(field) => (
                  <FormField label={t("detail.textBodyLabel")}>
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
      </CardContent>

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
    </Card>
  )
}

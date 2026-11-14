import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { TemplateView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { VisualBodyPreview } from "@/components/visual-editor/VisualBodyPreview"
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
// in visual mode for campaign-kind templates. Visual rows show a preview of
// the saved body and link to the full-screen editor. Pre-Phase-7 raw-HTML templates (body_doc == null
// AND body_html non-empty) stay in code-only mode — and transactional
// templates never enter the visual editor (per plan.md / spec.md scope).
function initialTemplateEditorMode(template: TemplateView): "visual" | "code" {
  if (template.kind === "transactional") return "code"
  if (template.body_doc) return "visual"
  if (template.body_html.trim() === "") return "visual"
  return "code"
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
  const { t } = useTranslation(["templates", "common", "visualEditor"])

  // Mode is mutable so the operator can opt out of the visual editor on a
  // visual row (T093 / FR-029). Transactional templates never enter
  // visual mode — the convert link is gated on kind === "campaign".
  const [editorMode, setEditorMode] = useState<"visual" | "code">(() =>
    initialTemplateEditorMode(template),
  )
  const [confirmOptOut, setConfirmOptOut] = useState(false)
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
    onSuccess: async (res) => {
      await invalidate()
      // Show what the sanitizer actually stored.
      form.setFieldValue("bodyHtml", res.data.body_html)
      const warnings = res.data.warnings?.length ?? 0
      if (warnings > 0) {
        toast.warning(t("detail.saveWarnings", { count: warnings }))
      } else {
        toast.success(t("detail.saveSuccess"))
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

  const form = useForm({
    defaultValues: {
      name: template.name,
      subject: template.subject,
      bodyHtml: template.body_html,
      bodyText: template.body_text,
    },
    onSubmit: async ({ value }) => {
      // In visual mode the body belongs to the full-screen editor; this form
      // saves name + subject only and sends the stored body back unchanged
      // (the legacy update never touches body_doc).
      const body =
        editorMode === "visual"
          ? { bodyHtml: template.body_html, bodyText: template.body_text }
          : {}
      await save.mutateAsync({ ...value, ...body }).catch(() => {})
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
          {editorMode === "visual" ? (
            <VisualBodyPreview
              html={template.body_html}
              actions={
                canManage && (
                  <>
                    <Button asChild size="sm">
                      <Link
                        to="/t/$slug/templates/$id/edit"
                        params={{ slug, id: template.id }}
                        data-testid="open-visual-editor"
                      >
                        {t("visualEditor:body.openEditor")}
                      </Link>
                    </Button>
                    {template.body_doc && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setConfirmOptOut(true)}
                        data-testid="ve-opt-out-visual"
                      >
                        {t("visualEditor:body.editHtmlOnly")}
                      </Button>
                    )}
                  </>
                )
              }
            />
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
                          <Button asChild variant="outline" size="sm">
                            <Link
                              to="/t/$slug/templates/$id/edit"
                              params={{ slug, id: template.id }}
                              data-testid="convert-to-visual"
                            >
                              {t("visual.convert")}
                            </Link>
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
              <Button type="submit" disabled={save.isPending}>
                {save.isPending ? t("detail.saving") : t("detail.save")}
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

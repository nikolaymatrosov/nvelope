import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { TemplateView, VisualDoc } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { ApiError, errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { VisualEmailEditor } from "@/components/visual-editor/VisualEmailEditor"
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
          ← Templates
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
      toast.success("Template saved.")
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
  // clears body_doc + theme so the row reverts to a code-only template.
  const optOutVisual = useMutation({
    mutationFn: () => api.templates.optOutVisual(slug, template.id),
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

  async function refetchTemplate(): Promise<TemplateView | undefined> {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.template(slug, template.id),
    })
    const fresh = (await api.getTemplate(slug, template.id)).data
    setBodyDoc(fresh.body_doc ?? EMPTY_VISUAL_DOC)
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
        theme: template.theme ?? null,
        ifUnmodifiedSince,
      }),
    onSuccess: async (res) => {
      setIfUnmodifiedSince(res.data.updatedAt)
      await invalidate()
      const warnings = res.data.warnings.length
      if (warnings > 0) {
        toast.warning(
          `Template saved with ${warnings} content warning${warnings === 1 ? "" : "s"}.`,
        )
      } else {
        toast.success("Template saved.")
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
              void refetchTemplate()
            },
          },
          cancel: {
            label: "Force overwrite",
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
                  theme: template.theme ?? null,
                  ifUnmodifiedSince: currentUpdatedAt,
                })
                .then(async (res) => {
                  setIfUnmodifiedSince(res.data.updatedAt)
                  await invalidate()
                  toast.success("Template saved.")
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
          <Badge variant="secondary">{template.kind}</Badge>
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
              onBlur: compose(rules.required("Enter a template name.")),
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
              <Label htmlFor="template-visual-editor">Body</Label>
              <VisualEmailEditor
                slug={slug}
                value={bodyDoc}
                onChange={setBodyDoc}
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
                      <Label>HTML body</Label>
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
                              ? "Converting…"
                              : "Convert to visual editor"}
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
      </CardContent>

      <Dialog open={confirmOptOut} onOpenChange={setConfirmOptOut}>
        <DialogContent data-testid="opt-out-visual-dialog">
          <DialogHeader>
            <DialogTitle>Switch to HTML-only editing?</DialogTitle>
            <DialogDescription>
              Your structured visual document will be discarded. The last
              saved HTML body stays intact so the template remains usable,
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
    </Card>
  )
}

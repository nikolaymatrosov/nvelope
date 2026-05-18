import { Link, createFileRoute } from "@tanstack/react-router"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { TemplateView } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Textarea } from "@/components/ui/textarea"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
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
      await queryClient.invalidateQueries({
        queryKey: queryKeys.templates(slug),
      })
      await queryClient.invalidateQueries({
        queryKey: queryKeys.template(slug, template.id),
      })
      toast.success("Template saved.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: {
      name: template.name,
      subject: template.subject,
      bodyHtml: template.body_html,
      bodyText: template.body_text,
    },
    onSubmit: async ({ value }) => {
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
          {canManage && (
            <div>
              <Button type="submit" disabled={save.isPending}>
                {save.isPending ? "Saving…" : "Save changes"}
              </Button>
            </div>
          )}
        </form>
      </CardContent>
    </Card>
  )
}

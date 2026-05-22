import { createFileRoute } from "@tanstack/react-router"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import type { WorkspaceSettings } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { AsyncState } from "@/components/common/async-state"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/settings/")({
  component: SettingsView,
})

export function SettingsView() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("settings")
  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(slug),
    queryFn: async () => (await api.getSettings(slug)).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("page.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("page.description")}
        </p>
      </header>

      <AsyncState query={settingsQuery}>
        {(settings) => <SettingsForm slug={slug} settings={settings} />}
      </AsyncState>
    </div>
  )
}

function SettingsForm({
  slug,
  settings,
}: {
  slug: string
  settings: WorkspaceSettings
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["settings", "common"])

  const save = useMutation({
    mutationFn: (v: WorkspaceSettings) => api.updateSettings(slug, v),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.settings(slug) })
      toast.success(t("form.savedToast"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: {
      display_name: settings.display_name,
      timezone: settings.timezone,
    },
    onSubmit: async ({ value }) => {
      await save.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("form.title")}</CardTitle>
        <CardDescription>{t("form.description")}</CardDescription>
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
            name="display_name"
            validators={{
              onBlur: compose(rules.required(t("form.displayName.required"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("form.displayName.label")}
                required
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <form.Field name="timezone">
            {(field) => (
              <FormField
                label={t("form.timezone.label")}
                hint={t("form.timezone.hint")}
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
              />
            )}
          </form.Field>
          <div>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? t("common:actions.saving") : t("form.submit")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

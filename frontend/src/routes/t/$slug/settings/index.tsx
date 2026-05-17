import { createFileRoute } from "@tanstack/react-router"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
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
  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(slug),
    queryFn: async () => (await api.getSettings(slug)).data,
  })

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Configure this workspace.
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

  const save = useMutation({
    mutationFn: (v: WorkspaceSettings) => api.updateSettings(slug, v),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.settings(slug) })
      toast.success("Settings saved.")
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
        <CardTitle>Workspace settings</CardTitle>
        <CardDescription>
          These values apply across the whole workspace.
        </CardDescription>
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
              onBlur: compose(rules.required("Enter a display name.")),
            }}
          >
            {(field) => (
              <FormField
                label="Display name"
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
                label="Timezone"
                hint="An IANA timezone, e.g. Europe/Berlin."
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
              />
            )}
          </form.Field>
          <div>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "Saving…" : "Save settings"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

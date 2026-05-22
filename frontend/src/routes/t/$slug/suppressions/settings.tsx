import { Link, createFileRoute } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeftIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { BounceSettings } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { AsyncState } from "@/components/common/async-state"

export const Route = createFileRoute("/t/$slug/suppressions/settings")({
  component: BounceSettingsPage,
})

export function BounceSettingsPage() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("suppressions")
  const { can } = usePermissions(slug)
  const canManage = can("sending:manage")

  const query = useQuery({
    queryKey: queryKeys.bounceSettings(slug),
    queryFn: async () => (await api.bounceSettings.get(slug)).data,
    retry: false,
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-col gap-2">
        <Button variant="ghost" size="sm" className="w-fit -ml-2" asChild>
          <Link to="/t/$slug/suppressions" params={{ slug }}>
            <ArrowLeftIcon /> {t("settings.back")}
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-semibold">{t("settings.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("settings.description")}
          </p>
        </div>
      </header>

      <AsyncState query={query}>
        {(settings) => (
          <BounceSettingsForm
            slug={slug}
            settings={settings}
            canManage={canManage}
          />
        )}
      </AsyncState>
    </div>
  )
}

function BounceSettingsForm({
  slug,
  settings,
  canManage,
}: {
  slug: string
  settings: BounceSettings
  canManage: boolean
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation("suppressions")
  const [draft, setDraft] = useState<BounceSettings>(settings)

  useEffect(() => {
    setDraft(settings)
  }, [settings])

  const dirty =
    draft.suppressOnHardBounce !== settings.suppressOnHardBounce ||
    draft.suppressOnComplaint !== settings.suppressOnComplaint

  const save = useMutation({
    mutationFn: () => api.bounceSettings.update(slug, draft),
    onSuccess: (res) => {
      queryClient.setQueryData(queryKeys.bounceSettings(slug), res.data)
      toast.success(t("settings.savedToast"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.hardBounceTitle")}</CardTitle>
          <CardDescription>
            {t("settings.hardBounceDescription")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Label className="flex items-center gap-2">
            <Checkbox
              checked={draft.suppressOnHardBounce}
              disabled={!canManage}
              onCheckedChange={(v) =>
                setDraft((d) => ({ ...d, suppressOnHardBounce: v === true }))
              }
            />
            {t("settings.hardBounceToggle")}
          </Label>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("settings.complaintTitle")}</CardTitle>
          <CardDescription>
            {t("settings.complaintDescription")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Label className="flex items-center gap-2">
            <Checkbox
              checked={draft.suppressOnComplaint}
              disabled={!canManage}
              onCheckedChange={(v) =>
                setDraft((d) => ({ ...d, suppressOnComplaint: v === true }))
              }
            />
            {t("settings.complaintToggle")}
          </Label>
        </CardContent>
      </Card>

      {canManage && (
        <div className="flex justify-end">
          <Button
            disabled={!dirty || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? t("settings.saving") : t("settings.save")}
          </Button>
        </div>
      )}
    </div>
  )
}

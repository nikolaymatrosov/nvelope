// Full-screen visual editor for a campaign (/t/$slug/campaigns/$id/edit).
// Only drafts are editable; any other status opens read-only. The trailing
// underscore keeps the route out of $id.tsx's tree.

import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { usePermissions } from "@/hooks/use-permissions"
import { AsyncState } from "@/components/common/async-state"
import { Button } from "@/components/ui/button"
import { EditorScreen } from "@/components/visual-editor/EditorScreen"

export const Route = createFileRoute("/t/$slug/campaigns/$id_/edit")({
  component: CampaignEditPage,
  staticData: { fullscreen: true },
})

export function CampaignEditPage() {
  const { slug, id } = Route.useParams()
  const { t } = useTranslation("visualEditor")
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)

  const queryFn = async () => (await api.getCampaign(slug, id)).data
  const query = useQuery({
    queryKey: queryKeys.campaign(slug, id),
    queryFn,
  })

  const onBack = () => {
    void navigate({ to: "/t/$slug/campaigns/$id", params: { slug, id } })
  }

  return (
    <AsyncState
      query={query}
      errorAction={
        <Button type="button" variant="outline" size="sm" onClick={onBack}>
          {t("screen.back")}
        </Button>
      }
    >
      {(campaign) => (
        <EditorScreen
          key={campaign.id}
          slug={slug}
          section={t("screen.breadcrumb.campaigns")}
          row={campaign}
          editable={can("campaigns:manage") && campaign.status === "draft"}
          onBack={onBack}
          convert={() => api.campaigns.convertToVisual(slug, id)}
          save={(v) => api.campaigns.saveVisual(slug, id, { subject: campaign.subject, ...v })}
          refetch={() => queryClient.fetchQuery({ queryKey: queryKeys.campaign(slug, id), queryFn, staleTime: 0 })}
          onSaved={async () => {
            await queryClient.invalidateQueries({ queryKey: queryKeys.campaign(slug, id) })
            await queryClient.invalidateQueries({ queryKey: queryKeys.campaigns(slug) })
          }}
        />
      )}
    </AsyncState>
  )
}

// Full-screen visual editor for a template (/t/$slug/templates/$id/edit). The
// trailing underscore keeps the route out of $id.tsx's tree (that page has no
// <Outlet/>). AppShell hides its chrome via staticData.fullscreen.
// Transactional templates stay code-only, so they never open here.

import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { usePermissions } from "@/hooks/use-permissions"
import { AsyncState } from "@/components/common/async-state"
import { Button } from "@/components/ui/button"
import { EditorScreen } from "@/components/visual-editor/EditorScreen"

export const Route = createFileRoute("/t/$slug/templates/$id_/edit")({
  component: TemplateEditPage,
  staticData: { fullscreen: true },
})

export function TemplateEditPage() {
  const { slug, id } = Route.useParams()
  const { t } = useTranslation("visualEditor")
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)

  const query = useQuery({
    queryKey: queryKeys.template(slug, id),
    queryFn: async () => (await api.getTemplate(slug, id)).data,
  })

  const onBack = () => {
    void navigate({ to: "/t/$slug/templates/$id", params: { slug, id } })
  }

  return (
    <AsyncState query={query}>
      {(tpl) =>
        tpl.kind === "transactional" ? (
          <div className="ve-screen__notice">
            <p>{t("screen.unavailable")}</p>
            <Button type="button" variant="outline" onClick={onBack}>
              {t("screen.back")}
            </Button>
          </div>
        ) : (
          <EditorScreen
            key={tpl.id}
            slug={slug}
            section={t("screen.breadcrumb.templates")}
            row={tpl}
            editable={can("campaigns:manage")}
            onBack={onBack}
            convert={() => api.templates.convertToVisual(slug, id)}
            save={(v) =>
              api.templates.saveVisual(slug, id, {
                name: tpl.name,
                kind: tpl.kind,
                subject: tpl.subject,
                ...v,
              })
            }
            refetch={async () => (await api.getTemplate(slug, id)).data}
            onSaved={async () => {
              await queryClient.invalidateQueries({ queryKey: queryKeys.templates(slug) })
              await queryClient.invalidateQueries({ queryKey: queryKeys.template(slug, id) })
            }}
          />
        )
      }
    </AsyncState>
  )
}

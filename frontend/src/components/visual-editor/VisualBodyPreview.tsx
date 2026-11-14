// VisualBodyPreview — the detail-page stand-in for the visual editor: a
// sandboxed preview of the saved body_html plus caller-supplied actions
// ("Open editor", "Edit as HTML only"). It shows the stored HTML as-is, so it
// costs no render call.

import { useTranslation } from "react-i18next"
import type { ReactNode } from "react"
import { usePreviewHtml } from "@/lib/media-urls"

export function VisualBodyPreview({
  slug,
  html,
  actions,
}: {
  slug: string
  html: string
  actions?: ReactNode
}) {
  const { t } = useTranslation("visualEditor")
  const previewHtml = usePreviewHtml(slug, html)
  return (
    <div className="flex flex-col gap-2" data-testid="ve-body-preview">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{t("body.previewTitle")}</span>
        <div className="flex items-center gap-2">{actions}</div>
      </div>
      {html.trim() === "" ? (
        <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t("body.previewEmpty")}
        </p>
      ) : (
        <iframe
          title={t("body.previewTitle")}
          sandbox=""
          srcDoc={previewHtml}
          className="h-96 w-full rounded-md border bg-white"
        />
      )}
    </div>
  )
}

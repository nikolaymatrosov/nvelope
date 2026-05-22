// PreviewIframe — desktop (600 px) / mobile (375 px) toggle. Calls
// `api.renderPreview` (tenant-scoped — shared by campaign and template
// editors per the 2026-05-20 N4 clarification) with the current bodyDoc,
// optional theme, and an optional sample subscriber payload, then loads
// the returned HTML into a sandboxed iframe via `srcdoc`. Concrete widths
// match FR-007 (600 / 375 px).

import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import type {
  RenderPreviewSample,
  Theme,
  VisualDoc,
} from "@/lib/api-types"
import { api } from "@/lib/api"

type Props = {
  slug: string
  doc: VisualDoc
  theme: Theme | null
  sample: RenderPreviewSample | null
}

const WIDTHS = { desktop: 600, mobile: 375 } as const

export function PreviewIframe({ slug, doc, theme, sample }: Props) {
  const { t } = useTranslation("visualEditor")
  const [viewport, setViewport] = useState<keyof typeof WIDTHS>("desktop")
  const iframeRef = useRef<HTMLIFrameElement | null>(null)

  // Debounce the doc changes so we don't call the BFF on every keystroke.
  // The 400-ms window is roughly the BFF's p95 render budget per plan.md
  // "Performance Goals".
  const [debouncedDoc, setDebouncedDoc] = useState(doc)
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedDoc(doc), 400)
    return () => clearTimeout(timer)
  }, [doc])

  const query = useQuery({
    queryKey: ["render-preview", slug, debouncedDoc, theme, sample],
    queryFn: async () =>
      (
        await api.renderPreview(slug, {
          bodyDoc: debouncedDoc,
          theme,
          sample,
        })
      ).data,
  })

  useEffect(() => {
    const html = query.data?.bodyHtml
    const iframe = iframeRef.current
    if (!iframe || html === undefined) return
    iframe.srcdoc = html
  }, [query.data])

  return (
    <div
      className="ve-preview"
      data-testid="ve-preview"
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 8,
        alignItems: "center",
      }}
    >
      <div
        role="tablist"
        aria-label={t("preview.viewportAriaLabel")}
        style={{ display: "flex", gap: 4 }}
      >
        <button
          type="button"
          role="tab"
          aria-selected={viewport === "desktop"}
          data-testid="ve-preview-desktop"
          onClick={() => setViewport("desktop")}
        >
          {t("preview.desktop", { width: WIDTHS.desktop })}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={viewport === "mobile"}
          data-testid="ve-preview-mobile"
          onClick={() => setViewport("mobile")}
        >
          {t("preview.mobile", { width: WIDTHS.mobile })}
        </button>
      </div>
      {query.isError ? (
        <p style={{ color: "#b91c1c" }} data-testid="ve-preview-error">
          {t("preview.error", { message: query.error.message })}
        </p>
      ) : null}
      <iframe
        ref={iframeRef}
        data-testid="ve-preview-iframe"
        title={t("preview.iframeTitle")}
        sandbox=""
        style={{
          width: WIDTHS[viewport],
          height: 600,
          border: "1px solid #e5e7eb",
          borderRadius: 4,
          background: "white",
        }}
      />
    </div>
  )
}

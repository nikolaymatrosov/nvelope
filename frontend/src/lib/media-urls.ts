// Helpers for media-library URLs in the admin SPA.

import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

// EMAIL_CONTENT_WIDTH is the email content column. Images display at most
// this wide; the library stores them at up to twice that for sharp
// rendering on high-density screens.
export const EMAIL_CONTENT_WIDTH = 600

// imageDisplayWidth is the `width` an image block gets on insert: the
// asset's natural width capped at the content column, or null when unknown.
export function imageDisplayWidth(naturalWidth?: number | null): number | null {
  if (!naturalWidth || naturalWidth <= 0) return null
  return Math.min(naturalWidth, EMAIL_CONTENT_WIDTH)
}

// swapLibraryUrls replaces each asset's library URL with its presigned
// preview URL. Preview iframes are sandboxed with an opaque origin, so the
// session cookie that library URLs need never reaches them from inside one.
export function swapLibraryUrls(
  html: string,
  assets: ReadonlyArray<{ url: string; preview_url: string }>,
): string {
  let out = html
  for (const a of assets) out = out.split(a.url).join(a.preview_url)
  return out
}

// usePreviewHtml returns html with library URLs swapped for the media
// listing's presigned URLs, for a sandboxed preview iframe. The listing is
// cached for 30 minutes, under the 1-hour signature. If the listing is
// unavailable, html is returned unchanged (images show as broken).
export function usePreviewHtml<T extends string | undefined>(slug: string, html: T): T {
  const media = useQuery({
    queryKey: queryKeys.media(slug),
    queryFn: async () => (await api.media.list(slug)).data,
    staleTime: 30 * 60 * 1000,
  })
  if (html === undefined) return html
  return swapLibraryUrls(html, media.data?.items ?? []) as T
}

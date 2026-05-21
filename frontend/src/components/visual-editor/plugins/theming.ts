// theming — derives the editor's in-canvas style defaults (CSS custom
// properties) from a Theme value and, when the row has no pinned theme,
// from the tenant's Phase 6 branding via the existing branding query
// (TanStack Query, query key from `queryKeys.branding(slug)`).
//
// This is the SPA-side mirror of the BFF's `themeFromBranding` (see
// frontend/src/server/routes/visual-save.ts) and the Go domain's
// `Theme.DefaultsFromBranding`. Keeping the three resolution paths in
// lockstep means a freshly inserted button uses the brand color whether
// it's rendered in the in-canvas editor, the preview iframe, or the final
// email (per FR-022 acceptance scenario 1).
//
// The plugin is intentionally not a TipTap extension — it is a pure React
// hook returning a `style` object the caller spreads onto the editor's
// outer container. That keeps theming reactive to branding changes
// (TanStack Query refetches → hook re-renders → style updates) without
// requiring a TipTap re-mount.

import { useQuery } from "@tanstack/react-query"
import type { Theme } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

// PlatformDefaultTheme mirrors Go's defaults in
// `internal/campaign/domain/theme.go::DefaultsFromBranding` and the BFF's
// `PlatformDefaultTheme` in `frontend/src/server/render/index.ts`. Keeping
// one canonical set per stack is the cross-stack drift cost noted in the
// US3 plan (Constitution VI).
export const PlatformDefaultTheme: Theme = {
  textColor: "#222222",
  linkColor: "#0066cc",
  buttonColor: "#0066cc",
  buttonTextColor: "#ffffff",
  fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
  containerWidth: 600,
}

// CSS color shape used by the BFF / Go validators. Pinned to the same regex
// so an invalid string from a corrupt branding row falls back to the
// platform default rather than smuggling into an inline-style attribute.
const cssColorRE =
  /^(#[0-9a-fA-F]{3,8}|rgba?\([\s\d.,%/]+\)|[a-zA-Z]{3,30})$/

function safeColor(value: string | null | undefined, fallback: string): string {
  if (!value) return fallback
  const trimmed = value.trim()
  if (!cssColorRE.test(trimmed)) return fallback
  return trimmed
}

// themeFromBrandingPrimary mirrors Go's `DefaultsFromBranding` — a partial
// branding row (only `primary_color`) is enough to derive a working theme;
// every other field falls back to the platform default. Exported so
// `ThemeControls` (T108) can show the resolved values the BFF will use at
// save-time even before the operator pins anything.
export function themeFromBrandingPrimary(primaryColor: string | null | undefined): Theme {
  const primary = safeColor(primaryColor, PlatformDefaultTheme.linkColor)
  return {
    textColor: PlatformDefaultTheme.textColor,
    linkColor: primary,
    buttonColor: primary,
    buttonTextColor: PlatformDefaultTheme.buttonTextColor,
    fontFamily: PlatformDefaultTheme.fontFamily,
    containerWidth: PlatformDefaultTheme.containerWidth,
  }
}

// editorCssVariables maps a resolved Theme to the CSS custom properties the
// editor's stylesheet (visual-editor.css) reads on `.ve-editor` and
// `.ve-button`. Returning a flat record (not a CSSStyleDeclaration) keeps
// the React style prop ergonomic.
export function editorCssVariables(theme: Theme): Record<string, string> {
  return {
    "--ve-text-color": theme.textColor,
    "--ve-link-color": theme.linkColor,
    "--ve-button-color": theme.buttonColor,
    "--ve-button-text-color": theme.buttonTextColor,
    "--ve-font-family": theme.fontFamily,
    "--ve-container-width": `${theme.containerWidth}px`,
  }
}

// useEditorTheme resolves the effective Theme for the in-canvas editor
// surface:
//   - When the row carries a pinned theme override, the override wins.
//   - Otherwise the hook reads the tenant's branding via the existing
//     `api.branding.get(slug)` and derives defaults from `primary_color`.
//   - While branding is loading the platform default is returned so the
//     editor renders immediately; the resolved theme replaces it on next
//     render (no flash since the queries cache per-slug).
//
// `pinned` is the row's persisted theme (null = inherit). Pass it straight
// through from the campaign / template route's GET response.
export function useEditorTheme(slug: string, pinned: Theme | null): {
  theme: Theme
  isInherited: boolean
  isBrandingLoading: boolean
} {
  const brandingQuery = useQuery({
    queryKey: queryKeys.branding(slug),
    queryFn: async () => (await api.branding.get(slug)).data,
    // The branding row exists for every tenant; a 404 here is exceptional.
    enabled: pinned === null,
  })

  if (pinned !== null) {
    return { theme: pinned, isInherited: false, isBrandingLoading: false }
  }
  const branding = brandingQuery.data
  const theme = branding
    ? themeFromBrandingPrimary(branding.primary_color)
    : PlatformDefaultTheme
  return {
    theme,
    isInherited: true,
    isBrandingLoading: brandingQuery.isLoading,
  }
}

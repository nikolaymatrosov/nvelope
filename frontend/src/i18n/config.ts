// Static i18n configuration — the supported locales, the default, and the
// cookie that carries the effective locale for SSR. See
// specs/015-app-i18n-language-switcher/contracts/frontend-i18n.md.

export const SUPPORTED_LOCALES = ["en", "ru"] as const

export type Locale = (typeof SUPPORTED_LOCALES)[number]

export const DEFAULT_LOCALE: Locale = "en"

// Cookie name shared by the Go API (which writes it on auth + locale change),
// the i18next language detector, and the SSR locale resolver.
export const LOCALE_COOKIE = "nv_locale"

// Text direction per locale. Both launch locales are LTR; this map is the
// single place to extend when an RTL locale is added.
export const localeDir: Record<Locale, "ltr" | "rtl"> = {
  en: "ltr",
  ru: "ltr",
}

// Human-readable language name, each shown in its own language.
export const localeLabel: Record<Locale, string> = {
  en: "English",
  ru: "Русский",
}

export function isSupportedLocale(value: unknown): value is Locale {
  return (
    typeof value === "string" &&
    (SUPPORTED_LOCALES as ReadonlyArray<string>).includes(value)
  )
}

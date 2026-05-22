import i18n from "i18next"
import { initReactI18next } from "react-i18next"
import LanguageDetector from "i18next-browser-languagedetector"

import { DEFAULT_LOCALE, LOCALE_COOKIE, SUPPORTED_LOCALES } from "./config"

// Translation catalogs are static, bundled assets. Every JSON file under
// locales/<lng>/ is one namespace, auto-discovered here so adding a namespace
// (a feature area) needs no wiring change in this file.
const enModules = import.meta.glob("../locales/en/*.json", {
  eager: true,
  import: "default",
})
const ruModules = import.meta.glob("../locales/ru/*.json", {
  eager: true,
  import: "default",
})

function toNamespaces(
  modules: Record<string, unknown>
): Record<string, Record<string, unknown>> {
  const out: Record<string, Record<string, unknown>> = {}
  for (const [path, mod] of Object.entries(modules)) {
    const ns = path.slice(path.lastIndexOf("/") + 1).replace(/\.json$/, "")
    out[ns] = mod as Record<string, unknown>
  }
  return out
}

const resources = {
  en: toNamespaces(enModules),
  ru: toNamespaces(ruModules),
}

export const defaultNS = "common"

// Namespaces present in the English (source-of-truth) catalogs.
export const namespaces = Object.keys(resources.en)

const isBrowser = typeof document !== "undefined"

if (!i18n.isInitialized) {
  // The browser language detector is client-only — it reads document.cookie
  // and navigator. On the server the locale is seeded explicitly (SSR).
  if (isBrowser) {
    i18n.use(LanguageDetector)
  }

  void i18n.use(initReactI18next).init({
    resources,
    fallbackLng: DEFAULT_LOCALE,
    supportedLngs: [...SUPPORTED_LOCALES],
    defaultNS,
    ns: namespaces,
    lng: isBrowser ? undefined : DEFAULT_LOCALE,
    // An empty string is treated as missing so the en fallback applies.
    returnEmptyString: false,
    interpolation: { escapeValue: false },
    react: { useSuspense: false },
    detection: {
      order: ["cookie", "navigator"],
      lookupCookie: LOCALE_COOKIE,
      caches: ["cookie"],
      // Persist the choice for a year so it survives across sessions.
      cookieMinutes: 525600,
    },
    saveMissing: import.meta.env.DEV,
    missingKeyHandler: import.meta.env.DEV
      ? (lngs, ns, key) => {
          // FR-010: a raw key must never reach the UI. In development this
          // surfaces loudly; in production the English fallback covers it.
          console.error(
            `[i18n] missing key "${ns}:${key}" for ${lngs.join(", ")}`
          )
        }
      : undefined,
  })
}

export default i18n

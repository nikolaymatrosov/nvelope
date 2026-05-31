// A self-contained i18next instance for the react-email templates. It is kept
// separate from the app-wide i18n module (./index.ts) on purpose: that module
// discovers catalogs with Vite's `import.meta.glob`, a compile-time macro only
// the Vite bundler understands. Email templates are also rendered by the
// react-email CLI (Next.js/Turbopack), so they import catalogs statically here.

import { createInstance } from "i18next"

import enEmails from "../locales/en/emails.json"
import ruEmails from "../locales/ru/emails.json"
import { DEFAULT_LOCALE, SUPPORTED_LOCALES } from "./config"

const i18n = createInstance()

void i18n.init({
  resources: {
    en: { emails: enEmails },
    ru: { emails: ruEmails },
  },
  fallbackLng: DEFAULT_LOCALE,
  supportedLngs: [...SUPPORTED_LOCALES],
  defaultNS: "emails",
  ns: ["emails"],
  lng: DEFAULT_LOCALE,
  // An empty string is treated as missing so the en fallback applies.
  returnEmptyString: false,
  interpolation: { escapeValue: false },
})

export default i18n

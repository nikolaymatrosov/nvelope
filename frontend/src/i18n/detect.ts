// Effective-locale resolution. Precedence (research.md D4):
//   1. signed-in DB preference  — applied by the caller after `me()` resolves
//   2. nv_locale cookie
//   3. browser (Accept-Language on the server, navigator on the client)
//   4. default (English)
// This module covers steps 2–4; step 1 is layered on at the call site.

import { DEFAULT_LOCALE, LOCALE_COOKIE, isSupportedLocale } from "./config"
import type { Locale } from "./config"

// Reduces a BCP-47 tag ("en-US") to a supported base locale, or undefined.
function toSupported(tag: string | undefined): Locale | undefined {
  if (!tag) return undefined
  const base = tag.trim().toLowerCase().split("-")[0]
  return isSupportedLocale(base) ? base : undefined
}

function firstSupported(tags: Array<string | undefined>): Locale | undefined {
  for (const tag of tags) {
    const match = toSupported(tag)
    if (match) return match
  }
  return undefined
}

// Reads a single cookie value out of a Cookie header / document.cookie string.
export function readCookie(
  name: string,
  cookieString: string | undefined
): string | undefined {
  if (!cookieString) return undefined
  for (const part of cookieString.split(";")) {
    const eq = part.indexOf("=")
    if (eq === -1) continue
    if (part.slice(0, eq).trim() === name) {
      return decodeURIComponent(part.slice(eq + 1).trim())
    }
  }
  return undefined
}

// Parses an Accept-Language header into tags ordered by descending q-weight.
export function parseAcceptLanguage(header: string | undefined): Array<string> {
  if (!header) return []
  return header
    .split(",")
    .map((part) => {
      const [tag, ...params] = part.trim().split(";")
      const q = params.map((p) => p.trim()).find((p) => p.startsWith("q="))
      return { tag: tag.trim(), q: q ? Number(q.slice(2)) || 0 : 1 }
    })
    .filter((entry) => entry.tag)
    .sort((a, b) => b.q - a.q)
    .map((entry) => entry.tag)
}

// Client-side resolution: nv_locale cookie → navigator languages → default.
export function resolveClientLocale(): Locale {
  if (typeof document === "undefined") return DEFAULT_LOCALE
  const fromCookie = toSupported(readCookie(LOCALE_COOKIE, document.cookie))
  if (fromCookie) return fromCookie
  const navLangs =
    typeof navigator !== "undefined" ? [...navigator.languages] : []
  return firstSupported(navLangs) ?? DEFAULT_LOCALE
}

// Server-side resolution: nv_locale cookie → Accept-Language → default.
export function resolveServerLocale(
  cookieHeader: string | undefined,
  acceptLanguage: string | undefined
): Locale {
  const fromCookie = toSupported(readCookie(LOCALE_COOKIE, cookieHeader))
  if (fromCookie) return fromCookie
  return firstSupported(parseAcceptLanguage(acceptLanguage)) ?? DEFAULT_LOCALE
}

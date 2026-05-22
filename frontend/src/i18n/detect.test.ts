import { describe, expect, it } from "vitest"

import {
  parseAcceptLanguage,
  readCookie,
  resolveClientLocale,
  resolveServerLocale,
} from "./detect"

describe("readCookie", () => {
  it("extracts a named cookie value", () => {
    expect(readCookie("nv_locale", "foo=1; nv_locale=ru; bar=2")).toBe("ru")
  })

  it("returns undefined when the cookie is absent", () => {
    expect(readCookie("nv_locale", "foo=1")).toBeUndefined()
    expect(readCookie("nv_locale", undefined)).toBeUndefined()
  })
})

describe("parseAcceptLanguage", () => {
  it("orders tags by descending q-weight", () => {
    expect(parseAcceptLanguage("de;q=0.5, ru;q=0.9, en;q=0.1")).toEqual([
      "ru",
      "de",
      "en",
    ])
  })

  it("treats a missing q as weight 1", () => {
    expect(parseAcceptLanguage("en-US, ru;q=0.9")).toEqual(["en-US", "ru"])
  })
})

describe("resolveServerLocale", () => {
  it("prefers the nv_locale cookie over the browser", () => {
    expect(resolveServerLocale("nv_locale=ru", "en-US")).toBe("ru")
  })

  it("falls back to the first supported Accept-Language tag", () => {
    expect(resolveServerLocale(undefined, "de-DE, ru;q=0.8")).toBe("ru")
  })

  it("returns the default when nothing matches", () => {
    expect(resolveServerLocale(undefined, "de-DE, fr;q=0.8")).toBe("en")
    expect(resolveServerLocale(undefined, undefined)).toBe("en")
  })

  it("ignores an unsupported cookie value", () => {
    expect(resolveServerLocale("nv_locale=de", "ru")).toBe("ru")
  })
})

describe("resolveClientLocale", () => {
  it("reads the locale from the nv_locale cookie", () => {
    document.cookie = "nv_locale=ru"
    expect(resolveClientLocale()).toBe("ru")
    document.cookie = "nv_locale=; max-age=0"
  })
})

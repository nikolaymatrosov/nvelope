import { describe, expect, it } from "vitest"
import i18next from "i18next"

import { DEFAULT_LOCALE } from "./config"
import i18n from "./index"

describe("missing-translation fallback (FR-010)", () => {
  it("falls back to English when a key is missing in Russian", async () => {
    const instance = i18next.createInstance()
    await instance.init({
      resources: {
        en: { common: { actions: { save: "Save" }, state: { loading: "Loading…" } } },
        ru: { common: { actions: { save: "Сохранить" } } },
      },
      lng: "ru",
      fallbackLng: "en",
      returnEmptyString: false,
    })

    // Present in Russian — the Russian value is used.
    expect(instance.t("common:actions.save")).toBe("Сохранить")
    // Absent in Russian — the English value fills in, not a raw key.
    expect(instance.t("common:state.loading")).toBe("Loading…")
  })

  it("treats an empty Russian string as missing and falls back", async () => {
    const instance = i18next.createInstance()
    await instance.init({
      resources: {
        en: { common: { actions: { save: "Save" } } },
        ru: { common: { actions: { save: "" } } },
      },
      lng: "ru",
      fallbackLng: "en",
      returnEmptyString: false,
    })

    expect(instance.t("common:actions.save")).toBe("Save")
  })

  it("configures the app instance with an English fallback", () => {
    const fallback = i18n.options.fallbackLng
    const chain = Array.isArray(fallback) ? fallback : [fallback]
    expect(chain).toContain(DEFAULT_LOCALE)
    expect(i18n.options.returnEmptyString).toBe(false)
  })
})

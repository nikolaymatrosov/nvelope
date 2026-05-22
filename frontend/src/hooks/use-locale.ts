// Active interface locale plus the means to change it. Changing the locale
// re-renders the app in place — no route navigation, no URL change (FR-005,
// FR-006). The i18next browser language-detector caches the choice to the
// nv_locale cookie; for a signed-in user the choice is also persisted to the
// account so it follows them across devices (FR-004).

import { useCallback, useEffect } from "react"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import type { Locale } from "@/i18n/config"
import { api } from "@/lib/api"
import { errorMessage } from "@/lib/errors"
import { queryKeys } from "@/lib/query"
import { useSession } from "@/hooks/use-session"
import {
  DEFAULT_LOCALE,
  SUPPORTED_LOCALES,
  isSupportedLocale,
} from "@/i18n/config"

export function useLocale() {
  const { i18n } = useTranslation()
  const queryClient = useQueryClient()
  const { user } = useSession()

  const active: Locale = isSupportedLocale(i18n.language)
    ? i18n.language
    : DEFAULT_LOCALE

  const setLocale = useCallback(
    async (next: Locale) => {
      if (next === active) return
      const previous = active

      // Apply immediately — the detector caches the choice to the cookie.
      await i18n.changeLanguage(next)

      // Signed-out visitors keep the cookie-only choice.
      if (!user) return

      try {
        await api.updateMyLocale(next)
        await queryClient.invalidateQueries({ queryKey: queryKeys.me() })
      } catch (e) {
        // Persistence failed — revert so the UI matches the stored value.
        await i18n.changeLanguage(previous)
        toast.error(errorMessage(e))
      }
    },
    [active, i18n, user, queryClient]
  )

  return { locale: active, setLocale, supportedLocales: SUPPORTED_LOCALES }
}

// useSyncAccountLocale applies a signed-in user's stored locale preference
// (the highest-precedence source) once `me()` has resolved. This is what
// carries the choice to a new device whose cookie does not yet hold it.
export function useSyncAccountLocale() {
  const { i18n } = useTranslation()
  const { user } = useSession()
  const stored = user?.locale ?? undefined

  useEffect(() => {
    if (stored && isSupportedLocale(stored) && stored !== i18n.language) {
      void i18n.changeLanguage(stored)
    }
  }, [stored, i18n])
}

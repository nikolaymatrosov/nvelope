import { createServerFn } from "@tanstack/react-start"
import { getRequestHeader } from "@tanstack/react-start/server"

import { resolveServerLocale } from "./detect"

// Resolves the request's effective locale on the server from the nv_locale
// cookie and the Accept-Language header (research.md D4). Used by the root
// route's beforeLoad to seed i18next before the SSR render, so the first
// paint is already in the user's language — no flash on hydration (FR-005).
export const getServerLocale = createServerFn({ method: "GET" }).handler(() => {
  return resolveServerLocale(
    getRequestHeader("cookie"),
    getRequestHeader("accept-language")
  )
})

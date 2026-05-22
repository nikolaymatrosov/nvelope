import { afterEach, describe, expect, it, vi } from "vitest"
import { act, cleanup, renderHook } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { createElement } from "react"
import { useLocale } from "./use-locale"
import type { ReactNode } from "react"

import { api } from "@/lib/api"
import i18n from "@/i18n"

const signedIn = {
  user: { id: "u1", name: "Ada", email: "ada@example.com", locale: null },
  tenants: [],
}
let session: { user: unknown; tenants: Array<unknown> } = signedIn

vi.mock("@/hooks/use-session", () => ({
  useSession: () => session,
}))
vi.mock("@/lib/api", () => ({
  api: { updateMyLocale: vi.fn() },
}))
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }))

const ok = <T>(data: T) => ({ status: 200, ok: true, data })

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return createElement(QueryClientProvider, { client }, children)
}

afterEach(async () => {
  cleanup()
  vi.clearAllMocks()
  session = signedIn
  await i18n.changeLanguage("en")
})

describe("useLocale", () => {
  it("changes the active language and persists it for a signed-in user", async () => {
    vi.mocked(api.updateMyLocale).mockResolvedValue(
      ok({
        user: { id: "u1", name: "Ada", email: "ada@example.com", locale: "ru" },
      })
    )
    const { result } = renderHook(() => useLocale(), { wrapper })

    await act(async () => {
      await result.current.setLocale("ru")
    })

    expect(i18n.language).toBe("ru")
    expect(api.updateMyLocale).toHaveBeenCalledWith("ru")
  })

  it("reverts the active language when persistence fails", async () => {
    vi.mocked(api.updateMyLocale).mockRejectedValue(new Error("save failed"))
    const { result } = renderHook(() => useLocale(), { wrapper })

    await act(async () => {
      await result.current.setLocale("ru")
    })

    // The stored value did not change, so the UI reverts to it.
    expect(i18n.language).toBe("en")
  })

  it("does not call the API for a signed-out visitor", async () => {
    session = { user: undefined, tenants: [] }
    const { result } = renderHook(() => useLocale(), { wrapper })

    await act(async () => {
      await result.current.setLocale("ru")
    })

    expect(i18n.language).toBe("ru")
    expect(api.updateMyLocale).not.toHaveBeenCalled()
  })
})

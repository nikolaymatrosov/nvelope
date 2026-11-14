import { afterEach, describe, expect, it, vi } from "vitest"
import { act, renderHook, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { toast } from "sonner"
import { useVisualSave } from "./useVisualSave"
import type { ReactNode } from "react"
import type { VisualDoc } from "@/lib/api-types"
import { ApiError } from "@/lib/errors"

vi.mock("sonner", () => ({ toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() } }))

afterEach(() => vi.clearAllMocks())

const doc = (text: string): VisualDoc => ({
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text }] }],
})

const initial = { bodyDoc: doc("a"), theme: null, updatedAt: "2026-01-01T00:00:00Z" }

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

const okSave = (updatedAt: string) =>
  vi.fn().mockResolvedValue({ data: { updatedAt, warnings: [] } })

const stale = () =>
  new ApiError(409, "stale_row", "stale", "/x", { kind: "stale_row", currentUpdatedAt: "2026-03-03T00:00:00Z" })

describe("useVisualSave", () => {
  it("tracks dirty state and saves the current snapshot with the row token", async () => {
    const save = okSave("2026-02-02T00:00:00Z")
    const onSaved = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn(), onSaved }), { wrapper })
    expect(result.current.dirty).toBe(false)
    act(() => result.current.setBodyDoc(doc("b")))
    expect(result.current.dirty).toBe(true)
    act(() => result.current.save())
    await waitFor(() => expect(result.current.dirty).toBe(false))
    expect(save).toHaveBeenCalledWith({ bodyDoc: doc("b"), theme: null, ifUnmodifiedSince: initial.updatedAt })
    expect(onSaved).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()
  })

  it("uses the token from the previous save on the next one", async () => {
    const save = okSave("2026-02-02T00:00:00Z")
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn() }), { wrapper })
    act(() => result.current.setBodyDoc(doc("b")))
    act(() => result.current.save())
    await waitFor(() => expect(result.current.dirty).toBe(false))
    act(() => result.current.setBodyDoc(doc("c")))
    act(() => result.current.save())
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save.mock.calls[1][0].ifUnmodifiedSince).toBe("2026-02-02T00:00:00Z")
  })

  it("starts dirty when asked (freshly converted document)", () => {
    const { result } = renderHook(
      () => useVisualSave({ initial, initiallyDirty: true, save: vi.fn(), refetch: vi.fn() }),
      { wrapper },
    )
    expect(result.current.dirty).toBe(true)
  })

  it("on 409 offers Force overwrite, which re-sends the operator's doc with the fresh token", async () => {
    const save = vi
      .fn()
      .mockRejectedValueOnce(stale())
      .mockResolvedValueOnce({ data: { updatedAt: "2026-03-04T00:00:00Z", warnings: [] } })
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn() }), { wrapper })
    act(() => result.current.setBodyDoc(doc("mine")))
    act(() => result.current.save())
    await waitFor(() => expect(toast.warning).toHaveBeenCalled())
    const opts = vi.mocked(toast.warning).mock.calls[0][1] as unknown as { cancel: { onClick: () => void } }
    act(() => opts.cancel.onClick())
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save.mock.calls[1][0]).toEqual({
      bodyDoc: doc("mine"),
      theme: null,
      ifUnmodifiedSince: "2026-03-03T00:00:00Z",
    })
  })

  it("on 409 offers Reload, which replaces the doc with the server copy", async () => {
    const save = vi.fn().mockRejectedValueOnce(stale())
    const refetch = vi.fn().mockResolvedValue({ bodyDoc: doc("server"), theme: null, updatedAt: "2026-03-03T00:00:00Z" })
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch }), { wrapper })
    act(() => result.current.setBodyDoc(doc("mine")))
    act(() => result.current.save())
    await waitFor(() => expect(toast.warning).toHaveBeenCalled())
    const opts = vi.mocked(toast.warning).mock.calls[0][1] as unknown as { action: { onClick: () => void } }
    act(() => opts.action.onClick())
    await waitFor(() => expect(result.current.bodyDoc).toEqual(doc("server")))
    expect(result.current.dirty).toBe(false)
  })

  it("toasts other errors", async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error("boom"))
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn() }), { wrapper })
    act(() => result.current.save())
    await waitFor(() => expect(toast.error).toHaveBeenCalled())
  })
})

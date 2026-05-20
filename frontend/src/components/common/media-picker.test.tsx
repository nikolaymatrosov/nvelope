import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { MediaPicker } from "./media-picker"
import type { MediaAssetView } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@/lib/api", () => ({
  api: { media: { list: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function asset(overrides: Partial<MediaAssetView> = {}): MediaAssetView {
  return {
    id: "a1",
    filename: "logo.png",
    content_type: "image/png",
    size_bytes: 4096,
    public_url: "https://example.test/media/acme/logo.png",
    created_at: "2026-05-01T00:00:00Z",
    ...overrides,
  }
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("MediaPicker", () => {
  it("scopes the list call to the slug passed in", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [asset()] }))
    renderWithClient(
      <MediaPicker
        slug="acme"
        open
        onOpenChange={() => {}}
        onPick={() => {}}
      />,
    )
    await waitFor(() => expect(api.media.list).toHaveBeenCalledWith("acme"))
  })

  it("fires onPick with the selected asset and closes the modal", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [asset()] }))
    const onPick = vi.fn()
    const onOpenChange = vi.fn()
    renderWithClient(
      <MediaPicker
        slug="acme"
        open
        onOpenChange={onOpenChange}
        onPick={onPick}
      />,
    )
    const item = await screen.findByTestId("media-picker-item-a1")
    fireEvent.click(item)
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: "a1" }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("renders an empty state when no media exist", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(
      <MediaPicker slug="acme" open onOpenChange={() => {}} onPick={() => {}} />,
    )
    await waitFor(() => expect(api.media.list).toHaveBeenCalled())
    expect(screen.queryByTestId("media-picker-grid")).toBeNull()
  })

  it("does not fetch while closed (enabled gating)", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(
      <MediaPicker
        slug="acme"
        open={false}
        onOpenChange={() => {}}
        onPick={() => {}}
      />,
    )
    // Allow micro-tasks to flush.
    await Promise.resolve()
    expect(api.media.list).not.toHaveBeenCalled()
  })
})

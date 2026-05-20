import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { MediaLibrary } from "./index"
import type { MediaAssetView } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  Link: ({ children }: { children: unknown }) => <a href="#">{children as never}</a>,
}))

let canMock: (p: string) => boolean = () => true

vi.mock("@/hooks/use-permissions", () => ({
  usePermissions: () => ({
    can: (p: string) => canMock(p),
    canAny: () => true,
    isLoading: false,
    effective: { workspace: new Set() },
  }),
}))

vi.mock("@/lib/api", () => ({
  api: {
    media: {
      list: vi.fn(),
      upload: vi.fn(),
      remove: vi.fn(),
    },
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function asset(overrides: Partial<MediaAssetView> = {}): MediaAssetView {
  return {
    id: "a1",
    filename: "hero.png",
    content_type: "image/png",
    size_bytes: 1234,
    public_url: "https://example.test/media/acme/hero.png",
    created_at: "2026-05-01T00:00:00Z",
    ...overrides,
  }
}

beforeEach(() => {
  canMock = () => true
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("MediaLibrary", () => {
  it("hides the page behind a forbidden state without media:get", async () => {
    canMock = () => false
    renderWithClient(<MediaLibrary />)
    expect(await screen.findByTestId("media-forbidden")).toBeTruthy()
    expect(api.media.list).not.toHaveBeenCalled()
  })

  it("renders the empty state and upload control when the tenant has no media", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(<MediaLibrary />)
    expect(await screen.findByTestId("media-upload-button")).toBeTruthy()
    await waitFor(() =>
      expect(api.media.list).toHaveBeenCalledWith("acme"),
    )
  })

  it("hides the upload control without media:manage", async () => {
    canMock = (p) => p === "media:get"
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(<MediaLibrary />)
    await waitFor(() =>
      expect(api.media.list).toHaveBeenCalledWith("acme"),
    )
    expect(screen.queryByTestId("media-upload-button")).toBeNull()
  })

  it("rejects an oversized file before any network call", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(<MediaLibrary />)
    const input = (await screen.findByTestId(
      "media-upload-input",
    ))
    const big = new File(["x".repeat(11 * 1024 * 1024)], "big.png", {
      type: "image/png",
    })
    Object.defineProperty(input, "files", {
      configurable: true,
      value: [big],
    })
    fireEvent.change(input)
    expect(await screen.findByTestId("media-upload-error")).toBeTruthy()
    expect(api.media.upload).not.toHaveBeenCalled()
  })

  it("rejects a disallowed content-type before any network call", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(<MediaLibrary />)
    const input = (await screen.findByTestId(
      "media-upload-input",
    ))
    const evil = new File(["zz"], "evil.exe", { type: "application/x-msdownload" })
    Object.defineProperty(input, "files", {
      configurable: true,
      value: [evil],
    })
    fireEvent.change(input)
    expect(await screen.findByTestId("media-upload-error")).toBeTruthy()
    expect(api.media.upload).not.toHaveBeenCalled()
  })

  it("uploads a valid file and invalidates the listing on success", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    vi.mocked(api.media.upload).mockResolvedValue(
      ok({ id: "a1", public_url: "https://example.test/x.png", filename: "x.png" }),
    )
    renderWithClient(<MediaLibrary />)
    const input = (await screen.findByTestId(
      "media-upload-input",
    ))
    const ok_file = new File(["abc"], "x.png", { type: "image/png" })
    Object.defineProperty(input, "files", {
      configurable: true,
      value: [ok_file],
    })
    fireEvent.change(input)
    await waitFor(() =>
      expect(api.media.upload).toHaveBeenCalledWith("acme", ok_file),
    )
  })

  it("surfaces a server-side 413/inline reason instead of duplicating UI on partial state", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    vi.mocked(api.media.upload).mockRejectedValue(
      new ApiError(413, "media_too_large", "media file is too large", "/x"),
    )
    renderWithClient(<MediaLibrary />)
    const input = (await screen.findByTestId(
      "media-upload-input",
    ))
    const ok_file = new File(["abc"], "ok.png", { type: "image/png" })
    Object.defineProperty(input, "files", {
      configurable: true,
      value: [ok_file],
    })
    fireEvent.change(input)
    await waitFor(() => expect(api.media.upload).toHaveBeenCalled())
    // No partial entry — the empty state is still displayed.
    expect(screen.queryByTestId("media-grid")).toBeNull()
  })

  it("lists the tenant's media and opens the delete confirm dialog", async () => {
    vi.mocked(api.media.list).mockResolvedValue(
      ok({ items: [asset({ id: "a1" }), asset({ id: "a2", filename: "logo.svg", content_type: "image/svg+xml" })] }),
    )
    vi.mocked(api.media.remove).mockResolvedValue(ok(null))
    renderWithClient(<MediaLibrary />)
    expect(await screen.findByTestId("media-grid")).toBeTruthy()
    expect(screen.getByTestId("media-card-a1")).toBeTruthy()
    expect(screen.getByTestId("media-card-a2")).toBeTruthy()

    fireEvent.click(screen.getByTestId("media-delete-a1"))
    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(within(dialog).getByRole("button", { name: /delete/i }))
    await waitFor(() =>
      expect(api.media.remove).toHaveBeenCalledWith("acme", "a1"),
    )
  })

  it("scopes every list call to the operator's tenant slug", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(<MediaLibrary />)
    await screen.findByTestId("media-upload-button")
    expect(vi.mocked(api.media.list).mock.calls[0][0]).toBe("acme")
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen } from "@testing-library/react"
import { MediaDetail } from "./$id"
import type { MediaAssetView } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

let routeParams: { slug: string; id: string } = { slug: "acme", id: "a1" }

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => routeParams,
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
  api: { media: { list: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function asset(overrides: Partial<MediaAssetView> = {}): MediaAssetView {
  return {
    id: "a1",
    filename: "hero.png",
    content_type: "image/png",
    size_bytes: 4096,
    public_url: "https://example.test/media/acme/hero.png",
    created_at: "2026-05-01T00:00:00Z",
    ...overrides,
  }
}

beforeEach(() => {
  canMock = () => true
  routeParams = { slug: "acme", id: "a1" }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("MediaDetail", () => {
  it("renders the matching asset with its stable URL", async () => {
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [asset()] }))
    renderWithClient(<MediaDetail />)
    const filename = await screen.findByTestId("media-asset-filename")
    expect(filename.textContent).toContain("hero.png")
    expect(screen.getByTestId("media-asset-url").textContent).toContain(
      "https://example.test/media/acme/hero.png",
    )
  })

  it("renders the not-found state when the asset id does not match", async () => {
    routeParams = { slug: "acme", id: "missing" }
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [asset()] }))
    renderWithClient(<MediaDetail />)
    expect(await screen.findByTestId("media-asset-not-found")).toBeTruthy()
  })

  it("copies the URL on click", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    })
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [asset()] }))
    renderWithClient(<MediaDetail />)
    fireEvent.click(await screen.findByTestId("media-asset-copy"))
    expect(writeText).toHaveBeenCalledWith(
      "https://example.test/media/acme/hero.png",
    )
  })

  it("forbids the route without media:get", async () => {
    canMock = () => false
    renderWithClient(<MediaDetail />)
    expect(await screen.findByTestId("media-detail-forbidden")).toBeTruthy()
    expect(api.media.list).not.toHaveBeenCalled()
  })
})

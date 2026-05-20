import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { BrandingView_ } from "./index"
import type { BrandingView } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

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
    branding: { get: vi.fn(), save: vi.fn() },
    media: { list: vi.fn() },
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const sampleBranding: BrandingView = {
  logo_url: "",
  primary_color: "#1A73E8",
  custom_css: "",
}

beforeEach(() => {
  canMock = () => true
  vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("BrandingView", () => {
  it("renders the form with the current branding loaded", async () => {
    vi.mocked(api.branding.get).mockResolvedValue(ok(sampleBranding))
    renderWithClient(<BrandingView_ />)
    const hex = await screen.findByTestId("primary-color-hex")
    expect(hex.getAttribute("value")).toBe("#1A73E8")
    expect(api.branding.get).toHaveBeenCalledWith("acme")
  })

  it("hides the form behind a forbidden state when the user lacks branding:manage", async () => {
    canMock = () => false
    renderWithClient(<BrandingView_ />)
    expect(await screen.findByTestId("branding-forbidden")).toBeTruthy()
    expect(api.branding.get).not.toHaveBeenCalled()
  })

  it("disables save when custom CSS exceeds the limit", async () => {
    const big = "a".repeat(20_000)
    vi.mocked(api.branding.get).mockResolvedValue(
      ok({ ...sampleBranding, custom_css: big }),
    )
    renderWithClient(<BrandingView_ />)
    const save = await screen.findByTestId("save-branding")
    expect(save.hasAttribute("disabled")).toBe(true)
    expect(screen.getByTestId("css-editor-counter").textContent).toMatch(/bytes/)
  })

  it("saves the form and updates the sanitized preview from the server response", async () => {
    vi.mocked(api.branding.get).mockResolvedValue(ok(sampleBranding))
    vi.mocked(api.branding.save).mockResolvedValue(
      ok({
        logo_url: "https://example.test/logo.png",
        primary_color: "#1A73E8",
        custom_css: ".hdr { color: red; }",
      }),
    )
    renderWithClient(<BrandingView_ />)
    const logoInput = (await screen.findByTestId("logo-url-input"))
    fireEvent.change(logoInput, {
      target: { value: "https://example.test/logo.png" },
    })
    fireEvent.click(screen.getByTestId("save-branding"))
    await waitFor(() =>
      expect(api.branding.save).toHaveBeenCalledWith("acme", {
        logo_url: "https://example.test/logo.png",
        primary_color: "#1A73E8",
        custom_css: "",
      }),
    )
    // Sanitized preview block updates from the save response.
    await waitFor(() => screen.getByTestId("css-editor-sanitized"))
  })
})

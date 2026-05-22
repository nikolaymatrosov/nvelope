import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { BounceSettingsPage } from "./settings"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  Link: ({ children }: { children: unknown }) => (
    <a href="#">{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    bounceSettings: { get: vi.fn(), update: vi.fn() },
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com", locale: null }, tenants: [] }),
  )
  vi.mocked(api.tenant).mockResolvedValue(
    ok({
      tenant: { name: "Acme" },
      members: [
        { user_id: "u1", email: "ann@ex.com", name: "Ann", role: "Owner" },
      ],
    }),
  )
  vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("BounceSettingsPage", () => {
  it("shows both toggles on by default", async () => {
    setupOwner()
    vi.mocked(api.bounceSettings.get).mockResolvedValue(
      ok({ suppressOnHardBounce: true, suppressOnComplaint: true }),
    )
    renderWithClient(<BounceSettingsPage />)
    const boxes = await screen.findAllByRole("checkbox")
    expect(boxes).toHaveLength(2)
    for (const box of boxes) {
      expect(box.getAttribute("data-state")).toBe("checked")
    }
  })

  it("saves a changed toggle", async () => {
    setupOwner()
    vi.mocked(api.bounceSettings.get).mockResolvedValue(
      ok({ suppressOnHardBounce: true, suppressOnComplaint: true }),
    )
    vi.mocked(api.bounceSettings.update).mockResolvedValue(
      ok({ suppressOnHardBounce: false, suppressOnComplaint: true }),
    )
    renderWithClient(<BounceSettingsPage />)
    const boxes = await screen.findAllByRole("checkbox")
    fireEvent.click(boxes[0])
    fireEvent.click(screen.getByRole("button", { name: /save settings/i }))
    await waitFor(() =>
      expect(api.bounceSettings.update).toHaveBeenCalledWith("acme", {
        suppressOnHardBounce: false,
        suppressOnComplaint: true,
      }),
    )
  })

  it("keeps the save button disabled until a change is made", async () => {
    setupOwner()
    vi.mocked(api.bounceSettings.get).mockResolvedValue(
      ok({ suppressOnHardBounce: true, suppressOnComplaint: true }),
    )
    renderWithClient(<BounceSettingsPage />)
    const save = await screen.findByRole("button", { name: /save settings/i })
    expect(save.hasAttribute("disabled")).toBe(true)
  })
})

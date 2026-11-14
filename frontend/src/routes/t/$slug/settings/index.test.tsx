import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { SettingsView } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
}))

let canMock: (p: string) => boolean = () => true

vi.mock("@/hooks/use-permissions", () => ({
  usePermissions: () => ({ can: (p: string) => canMock(p) }),
}))

vi.mock("@/lib/api", () => ({
  api: { getSettings: vi.fn(), updateSettings: vi.fn(), setRawHTMLSanitization: vi.fn() },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  canMock = () => true
})

describe("SettingsView", () => {
  it("loads settings and saves an update", async () => {
    vi.mocked(api.getSettings).mockResolvedValue(
      ok({ display_name: "Acme", timezone: "UTC", raw_html_sanitization: "standard" }),
    )
    vi.mocked(api.updateSettings).mockResolvedValue(ok(null))
    renderWithClient(<SettingsView />)

    const nameField = await screen.findByLabelText(/display name/i)
    fireEvent.change(nameField, { target: { value: "Acme Inc" } })
    fireEvent.click(screen.getByRole("button", { name: /save settings/i }))

    await waitFor(() =>
      expect(api.updateSettings).toHaveBeenCalledWith("acme", {
        display_name: "Acme Inc",
        timezone: "UTC",
      }),
    )
  })

  it("disables raw HTML sanitization only after the risk is accepted", async () => {
    vi.mocked(api.getSettings).mockResolvedValue(
      ok({ display_name: "Acme", timezone: "UTC", raw_html_sanitization: "standard" }),
    )
    vi.mocked(api.setRawHTMLSanitization).mockResolvedValue(ok({ raw_html_sanitization: "off" }))
    renderWithClient(<SettingsView />)

    fireEvent.click(await screen.findByRole("button", { name: /disable sanitization/i }))
    const confirm = await screen.findByRole("button", { name: /disable anyway/i })
    expect((confirm as HTMLButtonElement).disabled).toBe(true)

    fireEvent.click(screen.getByRole("checkbox"))
    expect((confirm as HTMLButtonElement).disabled).toBe(false)
    fireEvent.click(confirm)

    await waitFor(() =>
      expect(api.setRawHTMLSanitization).toHaveBeenCalledWith("acme", {
        level: "off",
        accept_risk: true,
      }),
    )
  })

  it("hides the sanitization controls without settings:manage", async () => {
    canMock = (p) => p !== "settings:manage"
    vi.mocked(api.getSettings).mockResolvedValue(
      ok({ display_name: "Acme", timezone: "UTC", raw_html_sanitization: "standard" }),
    )
    renderWithClient(<SettingsView />)

    await screen.findByLabelText(/display name/i)
    expect(screen.queryByRole("button", { name: /disable sanitization/i })).toBeNull()
  })
})

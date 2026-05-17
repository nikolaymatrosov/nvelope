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

vi.mock("@/lib/api", () => ({
  api: { getSettings: vi.fn(), updateSettings: vi.fn() },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("SettingsView", () => {
  it("loads settings and saves an update", async () => {
    vi.mocked(api.getSettings).mockResolvedValue(
      ok({ display_name: "Acme", timezone: "UTC" }),
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
})

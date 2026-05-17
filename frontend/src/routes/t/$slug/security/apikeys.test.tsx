import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { ApiKeysPanel } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => opts,
}))

vi.mock("@/lib/api", () => ({
  api: {
    listAPIKeys: vi.fn(),
    issueAPIKey: vi.fn(),
    revokeAPIKey: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("ApiKeysPanel", () => {
  it("issues a key and shows the secret exactly once", async () => {
    vi.mocked(api.listAPIKeys).mockResolvedValue(ok({ api_keys: [] }))
    vi.mocked(api.issueAPIKey).mockResolvedValue(
      ok({ id: "key-1", token: "sk_live_secret" }),
    )
    renderWithClient(<ApiKeysPanel slug="acme" />)

    fireEvent.click(
      await screen.findByRole("button", { name: /issue api key/i }),
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/key name/i), {
      target: { value: "CI key" },
    })
    fireEvent.click(within(dialog).getByRole("button", { name: /issue key/i }))

    await waitFor(() =>
      expect(api.issueAPIKey).toHaveBeenCalledWith("acme", "CI key", []),
    )
    expect(await screen.findByText("sk_live_secret")).toBeDefined()
    expect(screen.getByText(/shown only once/i)).toBeDefined()
  })

  it("revokes an existing key after confirmation", async () => {
    vi.mocked(api.listAPIKeys).mockResolvedValue(
      ok({
        api_keys: [
          {
            ID: "key-1",
            Name: "Old key",
            Permissions: [],
            CreatedAt: "2026-01-01T00:00:00Z",
            LastUsedAt: null,
            RevokedAt: null,
          },
        ],
      }),
    )
    vi.mocked(api.revokeAPIKey).mockResolvedValue(ok(null))
    renderWithClient(<ApiKeysPanel slug="acme" />)

    fireEvent.click(await screen.findByRole("button", { name: /revoke key/i }))
    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(within(dialog).getByRole("button", { name: /revoke key/i }))

    await waitFor(() =>
      expect(api.revokeAPIKey).toHaveBeenCalledWith("acme", "key-1"),
    )
  })
})

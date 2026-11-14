import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { CampaignEditPage } from "./$id_.edit"
import type { VisualDoc } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "camp-1" }),
  }),
  useNavigate: () => navigate,
  useBlocker: vi.fn(),
}))

vi.mock("@/lib/api", () => ({
  api: {
    getCampaign: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
    branding: { get: vi.fn() },
    campaigns: { saveVisual: vi.fn(), convertToVisual: vi.fn() },
  },
}))

// The TipTap canvas is covered by EditorScreen/VisualEmailEditor tests; here
// a stub exposes the doc size and an "edit" button that makes the doc dirty.
vi.mock("@/components/visual-editor/VisualEmailEditor", () => ({
  VisualEmailEditor: ({ value, onChange }: { value: VisualDoc; onChange: (d: VisualDoc) => void }) => (
    <div data-testid="visual-email-editor" data-doc-blocks={String(value.content.length)}>
      <button
        type="button"
        data-testid="stub-edit"
        onClick={() => onChange({ ...value, content: [...value.content, { type: "divider" }] })}
      >
        edit
      </button>
    </div>
  ),
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const visualDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "Hi" }] }],
}

function campaign(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "camp-1",
    name: "Spring Sale",
    subject: "Big news",
    body_html: "<p>Hi</p>",
    body_text: "Hi",
    body_doc: visualDoc,
    theme: null,
    status: "draft",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }
}

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com", locale: null }, tenants: [] }),
  )
  vi.mocked(api.tenant).mockResolvedValue(
    ok({ tenant: { name: "Acme" }, members: [{ user_id: "u1", email: "ann@ex.com", name: "Ann", role: "Owner" }] }),
  )
  vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("CampaignEditPage", () => {
  it("saves a draft through campaigns.saveVisual with the subject and row token", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()) as never)
    vi.mocked(api.campaigns.saveVisual).mockResolvedValue(
      ok({ updatedAt: "2026-01-02T00:00:00Z", warnings: [] }) as never,
    )
    renderWithClient(<CampaignEditPage />)
    fireEvent.click(await screen.findByTestId("stub-edit"))
    await waitFor(() => expect(screen.getByTestId<HTMLButtonElement>("ve-save").disabled).toBe(false))
    fireEvent.click(screen.getByTestId("ve-save"))
    await waitFor(() =>
      expect(api.campaigns.saveVisual).toHaveBeenCalledWith("acme", "camp-1", {
        subject: "Big news",
        bodyDoc: { ...visualDoc, content: [...visualDoc.content, { type: "divider" }] },
        theme: null,
        ifUnmodifiedSince: "2026-01-01T00:00:00Z",
      }),
    )
  })

  it("opens a non-draft campaign read-only", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign({ status: "sent" })) as never)
    renderWithClient(<CampaignEditPage />)
    expect(await screen.findByTestId("ve-readonly-badge")).toBeTruthy()
    expect(screen.queryByTestId("ve-save")).toBeNull()
  })

  it("goes back to the campaign detail page", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()) as never)
    renderWithClient(<CampaignEditPage />)
    fireEvent.click(await screen.findByTestId("ve-back"))
    expect(navigate).toHaveBeenCalledWith({ to: "/t/$slug/campaigns/$id", params: { slug: "acme", id: "camp-1" } })
  })

  it("offers a back link when the campaign fails to load", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockRejectedValue(new Error("forbidden"))
    renderWithClient(<CampaignEditPage />)
    await screen.findByTestId("async-error")
    fireEvent.click(screen.getByRole("button", { name: "Back" }))
    expect(navigate).toHaveBeenCalledWith({ to: "/t/$slug/campaigns/$id", params: { slug: "acme", id: "camp-1" } })
  })
})

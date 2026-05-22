import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { CampaignsView } from "./index"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "camp-1" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children, to }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"}>{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    listCampaigns: vi.fn(),
    createCampaign: vi.fn(),
    listTemplates: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const sampleCampaign = {
  id: "camp-1",
  name: "Spring Sale",
  subject: "Big news",
  body_html: "<p>Hi</p>",
  body_text: "Hi",
  from_name: "Acme",
  from_local_part: "news",
  status: "draft" as const,
  max_send_errors: 100,
  sent_count: 0,
  failed_count: 0,
  recipient_count: 0,
  list_ids: [],
  segments: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

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
  vi.mocked(api.listTemplates).mockResolvedValue(
    ok({ templates: [], total: 0 }),
  )
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("CampaignsView", () => {
  it("lists campaigns with their status", async () => {
    setupOwner()
    vi.mocked(api.listCampaigns).mockResolvedValue(
      ok({ campaigns: [sampleCampaign], total: 1 }),
    )
    renderWithClient(<CampaignsView />)
    expect(await screen.findByText("Spring Sale")).toBeDefined()
    expect(screen.getByText("draft")).toBeDefined()
  })

  it("shows an empty state when there are no campaigns", async () => {
    setupOwner()
    vi.mocked(api.listCampaigns).mockResolvedValue(
      ok({ campaigns: [], total: 0 }),
    )
    renderWithClient(<CampaignsView />)
    expect(await screen.findByText(/no campaigns yet/i)).toBeDefined()
  })

  it("creates a campaign through the dialog", async () => {
    setupOwner()
    vi.mocked(api.listCampaigns).mockResolvedValue(
      ok({ campaigns: [], total: 0 }),
    )
    vi.mocked(api.createCampaign).mockResolvedValue(ok(sampleCampaign))
    renderWithClient(<CampaignsView />)

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /new campaign/i }))[0],
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/name/i), {
      target: { value: "Summer" },
    })
    fireEvent.click(
      within(dialog).getByRole("button", { name: /create campaign/i }),
    )

    await waitFor(() =>
      expect(api.createCampaign).toHaveBeenCalledWith(
        "acme",
        expect.objectContaining({ name: "Summer" }),
      ),
    )
  })
})

import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { CampaignDetail } from "./$id"
import type { CampaignStatus } from "@/lib/api-types"
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
    getCampaign: vi.fn(),
    updateCampaign: vi.fn(),
    startCampaign: vi.fn(),
    pauseCampaign: vi.fn(),
    resumeCampaign: vi.fn(),
    cancelCampaign: vi.fn(),
    setCampaignArchive: vi.fn(),
    listSendingDomains: vi.fn(),
    listLists: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
    media: { list: vi.fn() },
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function campaign(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "camp-1",
    name: "Spring Sale",
    subject: "Big news",
    body_html: "<p>Hi</p>",
    body_text: "Hi",
    from_name: "Acme",
    from_local_part: "news",
    sending_domain_id: undefined,
    status: "draft" as CampaignStatus,
    max_send_errors: 5,
    sent_count: 0,
    failed_count: 0,
    recipient_count: 0,
    list_ids: [],
    segments: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }
}

const verifiedDomain = {
  id: "dom-1",
  domain: "mail.acme.test",
  status: "verified" as const,
  dkim_records: [],
  spf_record: "",
  dmarc_record: "",
  created_at: "2026-01-01T00:00:00Z",
}

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com" }, tenants: [] }),
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
  vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("CampaignDetail", () => {
  it("blocks starting a draft campaign with no verified domain", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)

    const startBtn = await screen.findByRole("button", {
      name: /start campaign/i,
    })
    expect(startBtn.hasAttribute("disabled")).toBe(true)
    expect(
      screen.getByText(/verified sending domain is required/i),
    ).toBeDefined()
  })

  it("starts a campaign after confirmation when a verified domain is selected", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(campaign({ sending_domain_id: "dom-1" })),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(
      ok({ domains: [verifiedDomain] }),
    )
    vi.mocked(api.startCampaign).mockResolvedValue(ok({ status: "running" }))
    renderWithClient(<CampaignDetail />)

    const startBtn = await screen.findByRole("button", {
      name: /start campaign/i,
    })
    await waitFor(() => expect(startBtn.hasAttribute("disabled")).toBe(false))
    fireEvent.click(startBtn)

    const dialog = await screen.findByRole("alertdialog")
    expect(api.startCampaign).not.toHaveBeenCalled()
    fireEvent.click(
      within(dialog).getByRole("button", { name: /start sending/i }),
    )
    await waitFor(() =>
      expect(api.startCampaign).toHaveBeenCalledWith("acme", "camp-1"),
    )
  })

  it("shows send progress and pause/cancel actions for a running campaign", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          status: "running",
          sent_count: 4,
          failed_count: 1,
          recipient_count: 10,
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)

    expect(await screen.findByText(/send progress/i)).toBeDefined()
    expect(screen.getByRole("button", { name: /^pause$/i })).toBeDefined()
    expect(
      screen.getByRole("button", { name: /cancel campaign/i }),
    ).toBeDefined()
  })

  it("surfaces an auto-paused campaign with its reason", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          status: "paused",
          failed_count: 5,
          max_send_errors: 5,
          recipient_count: 10,
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)

    expect(await screen.findByText(/auto-paused/i)).toBeDefined()
    expect(screen.getByRole("button", { name: /resume/i })).toBeDefined()
  })

  it("shows the archive-visible toggle on a finished campaign and round-trips it", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          status: "finished",
          archive_visible: false,
          recipient_count: 10,
          sent_count: 10,
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.setCampaignArchive).mockResolvedValue(ok({ visible: true }))
    renderWithClient(<CampaignDetail />)

    expect(await screen.findByTestId("archive-visibility-card")).toBeTruthy()
    const toggle = screen.getByTestId("archive-visible-toggle")
    expect(toggle.getAttribute("aria-checked")).toBe("false")
    fireEvent.click(toggle)
    await waitFor(() =>
      expect(api.setCampaignArchive).toHaveBeenCalledWith("acme", "camp-1", true),
    )
  })

  it("hides the archive toggle on a draft campaign", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)
    await screen.findByRole("button", { name: /start campaign/i })
    expect(screen.queryByTestId("archive-visibility-card")).toBeNull()
  })

  it("opens the media picker from the HTML body field on a draft campaign", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.media.list).mockResolvedValue(ok({ items: [] }))
    renderWithClient(<CampaignDetail />)

    const open = await screen.findByTestId("open-media-picker")
    fireEvent.click(open)
    // Picker opens; media list is fetched.
    await waitFor(() => expect(api.media.list).toHaveBeenCalledWith("acme"))
  })
})

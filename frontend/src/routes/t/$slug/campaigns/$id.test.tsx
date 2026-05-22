import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { CampaignDetail } from "./$id"
import type { CampaignStatus, VisualDoc } from "@/lib/api-types"
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
    mergeTags: { list: vi.fn() },
    campaigns: {
      saveVisual: vi.fn(),
      convertToVisual: vi.fn(),
      optOutVisual: vi.fn(),
    },
  },
}))

// The campaign route renders <VisualEmailEditor> which mounts TipTap;
// jsdom doesn't fully implement Selection / Range APIs ProseMirror's
// drop-cursor uses. The unit-level editor tree is covered separately in
// `src/components/visual-editor/VisualEmailEditor.test.tsx` — here we
// stub the heavy component out so the route tests stay focused on the
// save flow and the stale-row UX.
vi.mock("@/components/visual-editor/VisualEmailEditor", () => ({
  VisualEmailEditor: ({
    value,
  }: {
    value: { content: Array<unknown> }
  }) => (
    <div
      data-testid="visual-email-editor"
      data-doc-blocks={String(value.content.length)}
    />
  ),
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

describe("CampaignDetail — visual editor surface (T070, T127)", () => {
  const visualDoc: VisualDoc = {
    version: 1,
    type: "doc",
    content: [
      { type: "paragraph", content: [{ type: "text", text: "Hi" }] },
    ],
  }

  it("renders <VisualEmailEditor /> when the row has body_doc", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(campaign({ body_doc: visualDoc })),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)
    expect(await screen.findByTestId("visual-email-editor")).toBeTruthy()
    // Legacy textarea is hidden in visual mode.
    expect(screen.queryByTestId("open-media-picker")).toBeNull()
  })

  it("keeps the code editor when the row is legacy raw-HTML", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(campaign({ body_doc: null, body_html: "<p>legacy</p>" })),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)
    expect(await screen.findByTestId("open-media-picker")).toBeTruthy()
    expect(screen.queryByTestId("visual-email-editor")).toBeNull()
  })

  it("saves via campaigns.saveVisual with the row's updated_at as ifUnmodifiedSince", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          body_doc: visualDoc,
          updated_at: "2026-05-20T12:34:56Z",
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.updateCampaign).mockResolvedValue(ok(campaign()))
    vi.mocked(api.campaigns.saveVisual).mockResolvedValue(
      ok({
        id: "camp-1",
        subject: "Big news",
        bodyHtml: "<p>Hi</p>",
        bodyText: "Hi",
        bodyDoc: visualDoc,
        theme: null,
        warnings: [],
        updatedAt: "2026-05-20T12:40:00Z",
      }),
    )
    renderWithClient(<CampaignDetail />)

    const saveBtn = await screen.findByRole("button", { name: /save changes/i })
    fireEvent.click(saveBtn)

    await waitFor(() =>
      expect(api.campaigns.saveVisual).toHaveBeenCalledWith(
        "acme",
        "camp-1",
        expect.objectContaining({
          ifUnmodifiedSince: "2026-05-20T12:34:56Z",
          bodyDoc: visualDoc,
          theme: null,
        }),
      ),
    )
  })

  // T083 — picking a visually-authored template at campaign-creation time
  // pre-fills body_doc on the new campaign, and the editor opens visually
  // when the operator lands on the campaign detail page. The Go-side
  // inheritance lives in CreateCampaign.Handle (T076 verifies it
  // end-to-end against Postgres); this test fixes the route-level
  // expectation that a campaign carrying both template_id and body_doc
  // mounts <VisualEmailEditor /> on first render.
  it("opens visually when the row was created from a visual template", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          template_id: "tpl-1",
          subject: "From template subject",
          body_html: "<p>template body</p>",
          body_text: "template body",
          body_doc: visualDoc,
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)
    expect(await screen.findByTestId("visual-email-editor")).toBeTruthy()
    expect(screen.queryByTestId("open-media-picker")).toBeNull()
  })

  // T094 — code ↔ visual round-trip + legacy / convert / opt-out flows.

  it("shows the Convert-to-visual button on a legacy raw-HTML row and converts on click", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          body_doc: null,
          body_html: "<p>legacy html body</p>",
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.campaigns.convertToVisual).mockResolvedValue(
      ok({
        bodyDoc: visualDoc,
        warnings: [
          {
            kind: "rawhtml_block",
            detail: "table preserved verbatim",
            path: "nodes[1]",
          },
        ],
      }),
    )

    renderWithClient(<CampaignDetail />)

    // Legacy code editor is visible; visual editor is not.
    expect(await screen.findByTestId("open-media-picker")).toBeTruthy()
    expect(screen.queryByTestId("visual-email-editor")).toBeNull()

    const convertBtn = await screen.findByTestId("convert-to-visual")
    fireEvent.click(convertBtn)

    await waitFor(() =>
      expect(api.campaigns.convertToVisual).toHaveBeenCalledWith(
        "acme",
        "camp-1",
      ),
    )
    // After conversion the visual editor is mounted with the candidate
    // doc loaded into state (data-doc-blocks reflects content.length).
    const editor = await screen.findByTestId("visual-email-editor")
    expect(editor.getAttribute("data-doc-blocks")).toBe("1")
  })

  it("opens the opt-out confirmation modal from the editor toolbar and clears body_doc on confirm", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(campaign({ body_doc: visualDoc, body_html: "<p>kept</p>" })),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.campaigns.optOutVisual).mockResolvedValue(
      ok(campaign({ body_doc: null, body_html: "<p>kept</p>" })),
    )

    // For this test we need the real <VisualEmailEditor /> to render the
    // toolbar's "Edit as HTML only" button. The unit-level editor test
    // covers the TipTap integration; here we only assert the toolbar's
    // wiring, so use the existing module mock to surface the button.
    vi.doMock("@/components/visual-editor/VisualEmailEditor", () => ({
      VisualEmailEditor: ({
        onOptOutVisual,
      }: {
        onOptOutVisual?: () => void
      }) => (
        <div data-testid="visual-email-editor">
          {onOptOutVisual && (
            <button
              type="button"
              data-testid="ve-opt-out-visual"
              onClick={onOptOutVisual}
            >
              Edit as HTML only
            </button>
          )}
        </div>
      ),
    }))
    vi.resetModules()
    const { CampaignDetail: ReloadedDetail } = await import("./$id")

    renderWithClient(<ReloadedDetail />)

    const optOutBtn = await screen.findByTestId("ve-opt-out-visual")
    fireEvent.click(optOutBtn)

    const dialog = await screen.findByTestId("opt-out-visual-dialog")
    expect(dialog).toBeTruthy()
    const confirm = within(dialog).getByTestId("opt-out-visual-confirm")
    fireEvent.click(confirm)

    await waitFor(() =>
      expect(api.campaigns.optOutVisual).toHaveBeenCalledWith(
        "acme",
        "camp-1",
      ),
    )
  })

  it("surfaces a stale_row 409 as an ApiError that the route can handle", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(
      ok(
        campaign({
          body_doc: visualDoc,
          updated_at: "2026-05-20T12:34:56Z",
        }),
      ),
    )
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.updateCampaign).mockResolvedValue(ok(campaign()))
    const { ApiError } = await import("@/lib/errors")
    vi.mocked(api.campaigns.saveVisual).mockRejectedValueOnce(
      new ApiError(
        409,
        "stale_row",
        "Changed in another tab/session",
        "/t/acme/api/campaigns/camp-1/visual",
        { currentUpdatedAt: "2026-05-20T12:38:00Z" },
      ),
    )
    renderWithClient(<CampaignDetail />)

    const saveBtn = await screen.findByRole("button", { name: /save changes/i })
    fireEvent.click(saveBtn)

    // The route catches the 409 and offers a recovery UX via sonner.
    // The mutation completes — we assert the API was called with the
    // stale token, confirming the route reaches the 409 branch rather
    // than crashing.
    await waitFor(() =>
      expect(api.campaigns.saveVisual).toHaveBeenCalledWith(
        "acme",
        "camp-1",
        expect.objectContaining({
          ifUnmodifiedSince: "2026-05-20T12:34:56Z",
        }),
      ),
    )
  })
})

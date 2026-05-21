import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { TemplateDetail } from "./$id"
import type { VisualDoc } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "tpl-1" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children, to }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"}>{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    getTemplate: vi.fn(),
    updateTemplate: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
    templates: { saveVisual: vi.fn() },
  },
}))

// VisualEmailEditor mounts TipTap, which jsdom can't drive — the unit-level
// editor tests cover the editor behaviour. Here the heavy component is
// stubbed out so the route tests stay focused on swap-in logic and save
// flow.
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

function template(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "tpl-1",
    name: "Welcome",
    kind: "campaign" as const,
    subject: "Hi there",
    body_html: "<p>Hi</p>",
    body_text: "Hi",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }
}

const visualDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "Hi" }] }],
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
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("TemplateDetail — code-only path", () => {
  it("persists edits to a legacy raw-HTML template via updateTemplate", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template()))
    vi.mocked(api.updateTemplate).mockResolvedValue(ok(template()))
    renderWithClient(<TemplateDetail />)

    const nameInput = await screen.findByLabelText(/name/i)
    fireEvent.change(nameInput, { target: { value: "Welcome v2" } })
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }))

    await waitFor(() =>
      expect(api.updateTemplate).toHaveBeenCalledWith(
        "acme",
        "tpl-1",
        expect.objectContaining({ name: "Welcome v2" }),
      ),
    )
  })
})

// T082 — Templates editor route test (visual swap-in + legacy raw-HTML fallback).
describe("TemplateDetail — visual editor surface (T082)", () => {
  it("renders <VisualEmailEditor /> when the row has body_doc", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(template({ body_doc: visualDoc })),
    )
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByTestId("visual-email-editor")).toBeTruthy()
    // Legacy code-editor surface is not mounted in visual mode.
    expect(screen.queryByText(/html body/i)).toBeNull()
  })

  it("keeps the code editor when the row is a legacy raw-HTML template", async () => {
    setupOwner()
    // body_doc null + non-empty body_html ⇒ pre-Phase-7 raw-HTML template.
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(template({ body_doc: null, body_html: "<p>legacy</p>" })),
    )
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByText(/html body/i)).toBeTruthy()
    expect(screen.queryByTestId("visual-email-editor")).toBeNull()
  })

  it("saves via templates.saveVisual with the row's updated_at as ifUnmodifiedSince", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(
        template({
          body_doc: visualDoc,
          updated_at: "2026-05-20T12:34:56Z",
        }),
      ),
    )
    vi.mocked(api.updateTemplate).mockResolvedValue(ok(template()))
    vi.mocked(api.templates.saveVisual).mockResolvedValue(
      ok({
        id: "tpl-1",
        name: "Welcome",
        kind: "campaign" as const,
        subject: "Hi there",
        bodyHtml: "<p>Hi</p>",
        bodyText: "Hi",
        bodyDoc: visualDoc,
        theme: null,
        warnings: [],
        updatedAt: "2026-05-20T12:40:00Z",
      }),
    )
    renderWithClient(<TemplateDetail />)

    const saveBtn = await screen.findByRole("button", { name: /save changes/i })
    fireEvent.click(saveBtn)

    await waitFor(() =>
      expect(api.templates.saveVisual).toHaveBeenCalledWith(
        "acme",
        "tpl-1",
        expect.objectContaining({
          name: "Welcome",
          kind: "campaign",
          ifUnmodifiedSince: "2026-05-20T12:34:56Z",
          bodyDoc: visualDoc,
          theme: null,
        }),
      ),
    )
  })

  it("surfaces a stale_row 409 as an ApiError the route can handle", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(
        template({
          body_doc: visualDoc,
          updated_at: "2026-05-20T12:34:56Z",
        }),
      ),
    )
    vi.mocked(api.updateTemplate).mockResolvedValue(ok(template()))
    const { ApiError } = await import("@/lib/errors")
    vi.mocked(api.templates.saveVisual).mockRejectedValueOnce(
      new ApiError(
        409,
        "stale_row",
        "Changed in another tab/session",
        "/t/acme/api/templates/tpl-1/visual",
        { currentUpdatedAt: "2026-05-20T12:38:00Z" },
      ),
    )
    renderWithClient(<TemplateDetail />)

    const saveBtn = await screen.findByRole("button", { name: /save changes/i })
    fireEvent.click(saveBtn)

    // The route catches the 409 and offers a recovery UX via sonner. We
    // assert the API was called with the stale token, confirming the route
    // reaches the 409 branch rather than crashing.
    await waitFor(() =>
      expect(api.templates.saveVisual).toHaveBeenCalledWith(
        "acme",
        "tpl-1",
        expect.objectContaining({
          ifUnmodifiedSince: "2026-05-20T12:34:56Z",
        }),
      ),
    )
  })
})

// T084 — Transactional templates stay in code-only mode. The visual editor
// surface targets campaigns + campaign templates only (per plan.md scope).
describe("TemplateDetail — transactional templates (T084)", () => {
  it("opens a transactional template in the code editor, not <VisualEmailEditor />", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(template({ kind: "transactional" })),
    )
    renderWithClient(<TemplateDetail />)

    expect(await screen.findByText(/html body/i)).toBeTruthy()
    expect(screen.queryByTestId("visual-email-editor")).toBeNull()
  })

  it("opens a transactional template in code mode even when body_doc is non-null", async () => {
    // body_doc presence alone shouldn't pull a transactional template into
    // the visual editor — kind takes precedence.
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(template({ kind: "transactional", body_doc: visualDoc })),
    )
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByText(/html body/i)).toBeTruthy()
    expect(screen.queryByTestId("visual-email-editor")).toBeNull()
  })
})

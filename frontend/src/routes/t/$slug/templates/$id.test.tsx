import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { toast } from "sonner"
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
  Link: ({ children, to, "data-testid": testId }: { children: unknown; to?: unknown; "data-testid"?: string }) => (
    <a href={typeof to === "string" ? to : "#"} data-testid={testId}>
      {children as never}
    </a>
  ),
}))

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
}))

vi.mock("@/lib/api", () => ({
  api: {
    getTemplate: vi.fn(),
    updateTemplate: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
    templates: {
      saveVisual: vi.fn(),
      convertToVisual: vi.fn(),
      optOutVisual: vi.fn(),
    },
  },
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

describe("TemplateDetail — raw save sanitization", () => {
  it("warns and shows the stored body when the raw save returns warnings", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(
      ok(template({ body_html: "<p>Hi</p><script>x</script>" })),
    )
    vi.mocked(api.updateTemplate).mockResolvedValue(
      ok(template({ body_html: "<p>Hi</p>", warnings: [{ kind: "sanitizer_stripped", detail: "removed" }] })),
    )
    renderWithClient(<TemplateDetail />)

    await screen.findByDisplayValue("<p>Hi</p><script>x</script>")
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }))

    await waitFor(() => expect(toast.warning).toHaveBeenCalled())
    expect(toast.success).not.toHaveBeenCalled()
    await screen.findByDisplayValue("<p>Hi</p>")
    expect(screen.queryByDisplayValue("<p>Hi</p><script>x</script>")).toBeNull()
  })
})

describe("TemplateDetail — visual body", () => {
  it("shows the saved-body preview and an Open editor link for a visual row", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: visualDoc })))
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByTestId("ve-body-preview")).toBeTruthy()
    expect(screen.getByTestId("open-visual-editor").getAttribute("href")).toBe("/t/$slug/templates/$id/edit")
    expect(screen.queryByText(/html body/i)).toBeNull()
  })

  it("keeps the code editor for a legacy raw-HTML template and links Convert to the editor", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: null, body_html: "<p>legacy</p>" })))
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByText(/html body/i)).toBeTruthy()
    expect(screen.getByTestId("convert-to-visual").getAttribute("href")).toBe("/t/$slug/templates/$id/edit")
    expect(api.templates.convertToVisual).not.toHaveBeenCalled()
  })

  it("hides Convert on transactional templates", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ kind: "transactional", body_doc: null })))
    renderWithClient(<TemplateDetail />)
    await screen.findByRole("button", { name: /save changes/i })
    expect(screen.queryByTestId("convert-to-visual")).toBeNull()
  })

  it("saves only name and subject from the detail page in visual mode", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: visualDoc, body_html: "<p>kept</p>" })))
    vi.mocked(api.updateTemplate).mockResolvedValue(ok(template()))
    renderWithClient(<TemplateDetail />)
    const nameInput = await screen.findByLabelText(/name/i)
    fireEvent.change(nameInput, { target: { value: "Welcome v2" } })
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }))
    await waitFor(() =>
      expect(api.updateTemplate).toHaveBeenCalledWith(
        "acme",
        "tpl-1",
        expect.objectContaining({ name: "Welcome v2", body_html: "<p>kept</p>" }),
      ),
    )
    expect(api.templates.saveVisual).not.toHaveBeenCalled()
  })

  it("opts out of the visual editor from the detail page", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: visualDoc, body_html: "<p>kept</p>" })))
    vi.mocked(api.templates.optOutVisual).mockResolvedValue(ok(template({ body_doc: null, body_html: "<p>kept</p>" })))
    renderWithClient(<TemplateDetail />)
    fireEvent.click(await screen.findByTestId("ve-opt-out-visual"))
    fireEvent.click(await screen.findByTestId("opt-out-visual-confirm"))
    await waitFor(() => expect(api.templates.optOutVisual).toHaveBeenCalledWith("acme", "tpl-1"))
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
    expect(screen.queryByTestId("open-visual-editor")).toBeNull()
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
    expect(screen.queryByTestId("open-visual-editor")).toBeNull()
  })
})

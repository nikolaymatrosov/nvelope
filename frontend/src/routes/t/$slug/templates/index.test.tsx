import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { TemplatesView } from "./index"
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
    listTemplates: vi.fn(),
    createTemplate: vi.fn(),
    deleteTemplate: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const sampleTemplate = {
  id: "tpl-1",
  name: "Welcome",
  kind: "campaign" as const,
  subject: "Hi there",
  body_html: "<p>Hi</p>",
  body_text: "Hi",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
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

describe("TemplatesView", () => {
  it("shows an empty state when there are no templates", async () => {
    setupOwner()
    vi.mocked(api.listTemplates).mockResolvedValue(
      ok({ templates: [], total: 0 }),
    )
    renderWithClient(<TemplatesView />)
    expect(await screen.findByText(/no templates yet/i)).toBeDefined()
  })

  it("creates a template through the dialog", async () => {
    setupOwner()
    vi.mocked(api.listTemplates).mockResolvedValue(
      ok({ templates: [], total: 0 }),
    )
    vi.mocked(api.createTemplate).mockResolvedValue(ok(sampleTemplate))
    renderWithClient(<TemplatesView />)

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /new template/i }))[0],
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/name/i), {
      target: { value: "Welcome" },
    })
    fireEvent.change(within(dialog).getByLabelText(/subject/i), {
      target: { value: "Hi there" },
    })
    fireEvent.click(
      within(dialog).getByRole("button", { name: /create template/i }),
    )

    await waitFor(() =>
      expect(api.createTemplate).toHaveBeenCalledWith(
        "acme",
        expect.objectContaining({
          name: "Welcome",
          kind: "campaign",
          subject: "Hi there",
        }),
      ),
    )
  })

  it("requires confirmation before deleting a template", async () => {
    setupOwner()
    vi.mocked(api.listTemplates).mockResolvedValue(
      ok({ templates: [sampleTemplate], total: 1 }),
    )
    vi.mocked(api.deleteTemplate).mockResolvedValue(ok(null))
    renderWithClient(<TemplatesView />)

    fireEvent.click(
      await screen.findByRole("button", { name: /delete template/i }),
    )
    expect(api.deleteTemplate).not.toHaveBeenCalled()

    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(
      within(dialog).getByRole("button", { name: /delete template/i }),
    )
    await waitFor(() =>
      expect(api.deleteTemplate).toHaveBeenCalledWith("acme", "tpl-1"),
    )
  })
})

import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { TemplateDetail } from "./$id"
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

describe("TemplateDetail", () => {
  it("persists edits to the template", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(sampleTemplate))
    vi.mocked(api.updateTemplate).mockResolvedValue(ok(sampleTemplate))
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

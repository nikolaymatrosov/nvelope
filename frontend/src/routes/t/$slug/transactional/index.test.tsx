import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { TransactionalView } from "./index"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children, to }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"}>{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    listSendingDomains: vi.fn(),
    listAPIKeys: vi.fn(),
    issueAPIKey: vi.fn(),
    revokeAPIKey: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

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

describe("TransactionalView", () => {
  it("shows the endpoint reference", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.listAPIKeys).mockResolvedValue(ok({ api_keys: [] }))
    renderWithClient(<TransactionalView />)
    expect(await screen.findByText(/POST \/t\/acme\/api\/tx/)).toBeDefined()
  })

  it("warns when no verified domain exists", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(
      ok({
        domains: [
          {
            id: "d1",
            domain: "x.test",
            status: "pending",
            dkim_records: [],
            spf_record: "",
            dmarc_record: "",
            created_at: "2026-01-01T00:00:00Z",
          },
        ],
      }),
    )
    vi.mocked(api.listAPIKeys).mockResolvedValue(ok({ api_keys: [] }))
    renderWithClient(<TransactionalView />)
    expect(
      await screen.findByText(/verified sending domain is required/i),
    ).toBeDefined()
  })

  it("issues a transactional-scoped key and shows the secret once", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.listAPIKeys).mockResolvedValue(ok({ api_keys: [] }))
    vi.mocked(api.issueAPIKey).mockResolvedValue(
      ok({ id: "k1", token: "secret-token-xyz" }),
    )
    renderWithClient(<TransactionalView />)

    fireEvent.click(
      await screen.findByRole("button", { name: /issue api key/i }),
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/key name/i), {
      target: { value: "Integration" },
    })
    fireEvent.click(within(dialog).getByRole("button", { name: /issue key/i }))

    await waitFor(() =>
      expect(api.issueAPIKey).toHaveBeenCalledWith("acme", "Integration", [
        "transactional:send",
      ]),
    )
    expect(await screen.findByText("secret-token-xyz")).toBeDefined()
  })
})

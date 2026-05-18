import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { SendingDomainsView } from "./index"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "dom-1" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children, to }: { children: unknown; to?: unknown }) => (
    <a href={typeof to === "string" ? to : "#"}>{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    listSendingDomains: vi.fn(),
    addSendingDomain: vi.fn(),
    recheckSendingDomain: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const sampleDomain = {
  id: "dom-1",
  domain: "mail.acme.test",
  status: "pending" as const,
  dkim_records: [],
  spf_record: "v=spf1 include:postbox ~all",
  dmarc_record: "v=DMARC1; p=none",
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
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("SendingDomainsView", () => {
  it("shows an empty state when there are no domains", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<SendingDomainsView />)
    expect(await screen.findByText(/no sending domains yet/i)).toBeDefined()
  })

  it("lists domains with their status", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(
      ok({ domains: [sampleDomain] }),
    )
    renderWithClient(<SendingDomainsView />)
    expect(await screen.findByText("mail.acme.test")).toBeDefined()
    expect(screen.getByText("pending")).toBeDefined()
  })

  it("adds a domain through the dialog", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.addSendingDomain).mockResolvedValue(ok(sampleDomain))
    renderWithClient(<SendingDomainsView />)

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /add domain/i }))[0],
    )
    const dialog = await screen.findByRole("dialog")
    fireEvent.change(within(dialog).getByLabelText(/domain/i), {
      target: { value: "mail.acme.test" },
    })
    fireEvent.click(
      within(dialog).getByRole("button", { name: /add domain/i }),
    )

    await waitFor(() =>
      expect(api.addSendingDomain).toHaveBeenCalledWith(
        "acme",
        "mail.acme.test",
      ),
    )
  })

  it("triggers an immediate re-check for a pending domain", async () => {
    setupOwner()
    vi.mocked(api.listSendingDomains).mockResolvedValue(
      ok({ domains: [sampleDomain] }),
    )
    vi.mocked(api.recheckSendingDomain).mockResolvedValue(
      ok({ status: "pending" }),
    )
    renderWithClient(<SendingDomainsView />)

    fireEvent.click(await screen.findByRole("button", { name: /re-check/i }))
    await waitFor(() =>
      expect(api.recheckSendingDomain).toHaveBeenCalledWith("acme", "dom-1"),
    )
  })
})

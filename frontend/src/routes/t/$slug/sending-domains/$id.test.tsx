import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"
import { SendingDomainDetail } from "./$id"
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
    getSendingDomain: vi.fn(),
    recheckSendingDomain: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("SendingDomainDetail", () => {
  it("renders the DKIM, SPF, and DMARC records with copy actions", async () => {
    vi.mocked(api.getSendingDomain).mockResolvedValue(
      ok({
        id: "dom-1",
        domain: "mail.acme.test",
        status: "pending",
        dkim_records: [
          { type: "CNAME", name: "dk1._domainkey", value: "dk1.postbox" },
        ],
        spf_record: "v=spf1 include:postbox ~all",
        dmarc_record: "v=DMARC1; p=none",
        created_at: "2026-01-01T00:00:00Z",
      }),
    )
    renderWithClient(<SendingDomainDetail />)

    expect(await screen.findByText("dk1._domainkey")).toBeDefined()
    expect(screen.getByText("v=spf1 include:postbox ~all")).toBeDefined()
    expect(screen.getByText("v=DMARC1; p=none")).toBeDefined()
    expect(
      screen.getAllByRole("button", { name: /copy/i }).length,
    ).toBeGreaterThan(0)
  })

  it("shows an actionable reason for a failed domain", async () => {
    vi.mocked(api.getSendingDomain).mockResolvedValue(
      ok({
        id: "dom-1",
        domain: "mail.acme.test",
        status: "failed",
        dkim_records: [],
        spf_record: "v=spf1 ~all",
        dmarc_record: "v=DMARC1; p=none",
        failure_reason: "DKIM record not found at the published host.",
        created_at: "2026-01-01T00:00:00Z",
      }),
    )
    renderWithClient(<SendingDomainDetail />)

    expect(
      await screen.findByText(/dkim record not found/i),
    ).toBeDefined()
  })
})

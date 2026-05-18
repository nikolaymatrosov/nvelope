import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"
import { DashboardPage } from "./index"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  Link: ({ children, params }: { children: unknown; params?: { id?: string } }) => (
    <a href={`/analytics/${params?.id ?? ""}`}>{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: { analytics: { dashboard: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const zeroCounts = {
  sent: 0,
  delivered: 0,
  opened: 0,
  clicked: 0,
  bounced: 0,
  complained: 0,
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("DashboardPage", () => {
  it("renders totals, rates, and recent campaigns", async () => {
    vi.mocked(api.analytics.dashboard).mockResolvedValue(
      ok({
        totals: {
          sent: 500,
          delivered: 480,
          opened: 200,
          clicked: 50,
          bounced: 20,
          complained: 2,
        },
        deliverability: { bounceRate: 0.04, complaintRate: 0.004 },
        recentCampaigns: [
          {
            campaignId: "camp-1",
            name: "Spring Sale",
            sent: 500,
            openRate: 0.4,
            bounceRate: 0.04,
            complaintRate: 0.004,
          },
        ],
      }),
    )
    renderWithClient(<DashboardPage />)
    expect(await screen.findByText("Spring Sale")).toBeDefined()
    expect(screen.getByText("480")).toBeDefined()
    expect(screen.getByText("40%")).toBeDefined()
    expect(screen.getAllByText("4%").length).toBeGreaterThan(0)
  })

  it("links a recent campaign to its analytics view", async () => {
    vi.mocked(api.analytics.dashboard).mockResolvedValue(
      ok({
        totals: { ...zeroCounts, sent: 10 },
        deliverability: { bounceRate: 0, complaintRate: 0 },
        recentCampaigns: [
          {
            campaignId: "camp-9",
            name: "Newsletter",
            sent: 10,
            openRate: 0,
            bounceRate: 0,
            complaintRate: 0,
          },
        ],
      }),
    )
    renderWithClient(<DashboardPage />)
    const link = await screen.findByRole("link", { name: /newsletter/i })
    expect(link.getAttribute("href")).toBe("/analytics/camp-9")
  })

  it("shows an empty state with no sending activity", async () => {
    vi.mocked(api.analytics.dashboard).mockResolvedValue(
      ok({
        totals: zeroCounts,
        deliverability: { bounceRate: 0, complaintRate: 0 },
        recentCampaigns: [],
      }),
    )
    renderWithClient(<DashboardPage />)
    expect(await screen.findByText(/no sending activity yet/i)).toBeDefined()
  })
})

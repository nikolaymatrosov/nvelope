import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"
import { CampaignAnalyticsView } from "./$id.analytics"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "camp-1" }),
  }),
  Link: ({ children }: { children: unknown }) => (
    <a href="#">{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: { analytics: { campaign: vi.fn() } },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("CampaignAnalyticsView", () => {
  it("renders counts and rates", async () => {
    vi.mocked(api.analytics.campaign).mockResolvedValue(
      ok({
        campaignId: "camp-1",
        counts: {
          sent: 100,
          delivered: 95,
          opened: 40,
          clicked: 10,
          bounced: 5,
          complained: 1,
        },
        rates: {
          openRate: 0.42,
          clickRate: 0.1,
          bounceRate: 0.05,
          complaintRate: 0.01,
        },
        refreshedAt: "2026-05-18T10:00:00Z",
      }),
    )
    renderWithClient(<CampaignAnalyticsView />)
    expect(await screen.findByText("Delivered")).toBeDefined()
    expect(screen.getByText("95")).toBeDefined()
    expect(screen.getByText("42%")).toBeDefined()
    expect(screen.getByText(/last refreshed/i)).toBeDefined()
  })

  it("shows an awaiting-data state before the first refresh", async () => {
    vi.mocked(api.analytics.campaign).mockResolvedValue(
      ok({
        campaignId: "camp-1",
        counts: {
          sent: 100,
          delivered: 0,
          opened: 0,
          clicked: 0,
          bounced: 0,
          complained: 0,
        },
        rates: {
          openRate: 0,
          clickRate: 0,
          bounceRate: 0,
          complaintRate: 0,
        },
        refreshedAt: null,
      }),
    )
    renderWithClient(<CampaignAnalyticsView />)
    expect(await screen.findByTestId("analytics-awaiting")).toBeDefined()
    expect(screen.getByText("100")).toBeDefined()
    expect(screen.getAllByText("0%").length).toBeGreaterThan(0)
  })

  it("shows a not-found state for an unknown campaign", async () => {
    vi.mocked(api.analytics.campaign).mockRejectedValue(
      new ApiError(404, "campaign-not-found", "Not found", "/x"),
    )
    renderWithClient(<CampaignAnalyticsView />)
    expect(await screen.findByTestId("analytics-not-found")).toBeDefined()
  })
})

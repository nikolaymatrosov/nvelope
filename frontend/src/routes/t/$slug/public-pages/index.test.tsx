import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen } from "@testing-library/react"
import { PublicPagesView } from "./index"
import type { SubscriptionPageView } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  Link: ({ children, params, ...rest }: { children: unknown } & Record<string, unknown>) => {
    void params
    return <a {...rest}>{children as never}</a>
  },
}))

vi.mock("@/hooks/use-permissions", () => ({
  usePermissions: () => ({
    can: (p: string) => p === "subscription_pages:manage",
    canAny: () => true,
    isLoading: false,
    effective: { workspace: new Set() },
  }),
}))

vi.mock("@/lib/api", () => ({
  api: {
    subscriptionPages: { list: vi.fn() },
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function page(overrides: Partial<SubscriptionPageView> = {}): SubscriptionPageView {
  return {
    ID: "p1",
    Slug: "newsletter",
    Title: "Newsletter",
    TargetListIDs: ["l1"],
    Fields: [],
    SendingDomainID: "d1",
    FromName: "Acme",
    FromLocalPart: "hello",
    Active: true,
    CreatedAt: "2026-05-01T00:00:00Z",
    UpdatedAt: "2026-05-01T00:00:00Z",
    ...overrides,
  }
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("PublicPagesView", () => {
  it("shows the empty state and create CTA when no pages exist", async () => {
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [] }),
    )
    renderWithClient(<PublicPagesView />)
    expect(
      await screen.findByTestId("create-first-subscription-page"),
    ).toBeTruthy()
    expect(api.subscriptionPages.list).toHaveBeenCalledWith("acme")
  })

  it("lists pages and renders the public URL bundle", async () => {
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({
        subscription_pages: [
          page({ ID: "p1", Slug: "newsletter", Title: "Newsletter" }),
          page({
            ID: "p2",
            Slug: "events",
            Title: "Events",
            Active: false,
          }),
        ],
      }),
    )
    renderWithClient(<PublicPagesView />)
    expect(await screen.findByTestId("subscription-page-row-p1")).toBeTruthy()
    expect(screen.getByTestId("subscription-page-row-p2")).toBeTruthy()
    // Bundle includes preference template, archive, and RSS (always).
    expect(screen.getByTestId("public-url-row-preference-template")).toBeTruthy()
    expect(screen.getByTestId("public-url-row-archive")).toBeTruthy()
    expect(screen.getByTestId("public-url-row-rss")).toBeTruthy()
    // Only active subscription pages appear as subscription rows.
    expect(screen.getAllByTestId(/public-url-row-subscription/)).toHaveLength(1)
  })

  it("scopes the list call to the operator's tenant slug", async () => {
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [] }),
    )
    renderWithClient(<PublicPagesView />)
    await screen.findByTestId("create-first-subscription-page")
    // The first argument is the slug — confirm it matches "acme" and never
    // leaks across tenants.
    expect(vi.mocked(api.subscriptionPages.list).mock.calls[0][0]).toBe("acme")
  })

  it("copies a public URL to the clipboard when the user clicks Copy", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    })
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({
        subscription_pages: [page({ ID: "p1", Slug: "newsletter" })],
      }),
    )
    renderWithClient(<PublicPagesView />)
    const row = await screen.findByTestId("public-url-row-subscription")
    const copyButton = row.querySelector('button[aria-label^="Copy"]')
    expect(copyButton).toBeTruthy()
    fireEvent.click(copyButton!)
    expect(writeText).toHaveBeenCalled()
    expect(writeText.mock.calls[0][0]).toContain("/t/acme/subscribe/newsletter")
  })
})

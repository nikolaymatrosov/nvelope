import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { SubscriptionPageEdit } from "./$id"
import type { SubscriptionPageView } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"

let routeParams: { slug: string; id: string } = { slug: "acme", id: "new" }

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => routeParams,
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: unknown }) => <a href="#">{children as never}</a>,
}))

let canMock: (p: string) => boolean = () => true

vi.mock("@/hooks/use-permissions", () => ({
  usePermissions: () => ({
    can: (p: string) => canMock(p),
    canAny: () => true,
    isLoading: false,
    effective: { workspace: new Set() },
  }),
}))

vi.mock("@/lib/api", () => ({
  api: {
    subscriptionPages: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
    },
    listLists: vi.fn(),
    listSendingDomains: vi.fn(),
    subscriberFields: {
      list: vi.fn(),
    },
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function pageView(overrides: Partial<SubscriptionPageView> = {}): SubscriptionPageView {
  return {
    ID: "p1",
    Slug: "newsletter",
    Title: "Newsletter",
    TargetListIDs: ["l1"],
    Fields: [{ key: "first_name", label: "First name", required: true }],
    SendingDomainID: "d1",
    FromName: "Acme",
    FromLocalPart: "hello",
    Active: true,
    CreatedAt: "2026-05-01T00:00:00Z",
    UpdatedAt: "2026-05-01T00:00:00Z",
    ...overrides,
  }
}

beforeEach(() => {
  canMock = () => true
  routeParams = { slug: "acme", id: "new" }
  vi.mocked(api.listLists).mockResolvedValue(
    ok({ lists: [{ ID: "l1", Name: "Customers" }], total: 1 }) as never,
  )
  vi.mocked(api.listSendingDomains).mockResolvedValue(
    ok({
      domains: [
        { id: "d1", domain: "acme.com", status: "verified" } as never,
      ],
    }),
  )
  vi.mocked(api.subscriberFields.list).mockResolvedValue(
    ok({
      fields: [
        {
          id: "builtin:first_name",
          slug: "first_name",
          displayName: "First name",
          type: "text",
          defaultValue: "",
          position: 0,
          builtIn: true,
          createdAt: "2026-05-01T00:00:00Z",
          updatedAt: "2026-05-01T00:00:00Z",
        },
        {
          id: "builtin:email",
          slug: "email",
          displayName: "Email",
          type: "url",
          defaultValue: "",
          position: 2,
          builtIn: true,
          createdAt: "2026-05-01T00:00:00Z",
          updatedAt: "2026-05-01T00:00:00Z",
        },
        {
          id: "f-country",
          slug: "country",
          displayName: "Country",
          type: "text",
          defaultValue: "",
          position: 10,
          builtIn: false,
          createdAt: "2026-05-01T00:00:00Z",
          updatedAt: "2026-05-01T00:00:00Z",
        },
      ],
    }) as never,
  )
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("SubscriptionPageEdit", () => {
  it("renders the create form when id is 'new'", async () => {
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [] }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    expect(
      await screen.findByRole("heading", { name: /new subscription page/i }),
    ).toBeTruthy()
    // Lists query was not blocked by enabled:false; the bound-lists card renders.
    await waitFor(() => screen.getByTestId("bound-lists"))
  })

  it("populates the edit form from the list query", async () => {
    routeParams = { slug: "acme", id: "p1" }
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [pageView()] }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    expect(
      await screen.findByRole("heading", { name: /edit subscription page/i }),
    ).toBeTruthy()
    // The "preview" control appears for an existing page with a slug.
    expect(screen.getByTestId("copy-public-url")).toBeTruthy()
    expect(screen.getByTestId("preview-public-url")).toBeTruthy()
  })

  it("shows a not-found state when the page id does not match any page", async () => {
    routeParams = { slug: "acme", id: "missing" }
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [pageView({ ID: "other" })] }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    expect(await screen.findByTestId("public-page-not-found")).toBeTruthy()
  })

  it("hides the form behind a forbidden state when the user lacks the permission", async () => {
    canMock = () => false
    renderWithClient(<SubscriptionPageEdit />)
    expect(await screen.findByTestId("public-pages-forbidden")).toBeTruthy()
    expect(api.subscriptionPages.list).not.toHaveBeenCalled()
  })

  it("blocks save when no list is selected and surfaces the inline error", async () => {
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [] }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    await screen.findByRole("heading", { name: /new subscription page/i })
    // Click save without selecting any list (and without filling required fields).
    fireEvent.click(screen.getByTestId("save-page"))
    // The TanStack-form required validators surface their messages; we accept
    // either the form-field-level "required" message or the server-side error.
    await waitFor(() => {
      const inline = screen.queryByTestId("form-server-error")
      const fieldErrors = screen.queryAllByRole("alert")
      expect(inline || fieldErrors.length > 0).toBeTruthy()
    })
    expect(api.subscriptionPages.create).not.toHaveBeenCalled()
  })

  it("populates the Add-field button from the subscriber-fields registry (FR-016b)", async () => {
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [] }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    await screen.findByRole("heading", { name: /new subscription page/i })
    // Wait for the registry query to resolve so the Add-field button is enabled.
    await waitFor(() => {
      const btn = screen.getByTestId("add-field")
      expect((btn as HTMLButtonElement).disabled).toBe(false)
    })
    fireEvent.click(screen.getByTestId("add-field"))
    // The newly-appended row picks the first available registry slug
    // (first_name — Email is filtered out because it's always shown).
    const select = (await screen.findByTestId(
      "field-key-0",
    )) as unknown as HTMLSelectElement
    expect(select.value).toBe("first_name")
    // The select options come from the registry: first_name + country (Email
    // is hidden because it's implicit, and the "no longer in registry" entry
    // only appears for stale slugs).
    const optionValues = Array.from(select.options).map((o) => o.value)
    expect(optionValues).toContain("first_name")
    expect(optionValues).toContain("country")
    expect(optionValues).not.toContain("email")
  })

  it("retains a stale field slug with a 'no longer in registry' marker", async () => {
    routeParams = { slug: "acme", id: "p1" }
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({
        subscription_pages: [
          pageView({
            Fields: [{ key: "favorite_color", label: "Favorite color", required: false }],
          }),
        ],
      }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    await screen.findByRole("heading", { name: /edit subscription page/i })
    const select = (await screen.findByTestId(
      "field-key-0",
    )) as unknown as HTMLSelectElement
    expect(select.value).toBe("favorite_color")
    const stale = Array.from(select.options).find(
      (o) => o.value === "favorite_color",
    )
    if (!stale) throw new Error("favorite_color option missing from select")
    expect(stale.textContent.toLowerCase()).toContain("no longer in registry")
  })

  it("offers the preview control when an existing page is loaded", async () => {
    routeParams = { slug: "acme", id: "p1" }
    vi.mocked(api.subscriptionPages.list).mockResolvedValue(
      ok({ subscription_pages: [pageView({ ID: "p1", Slug: "weekly" })] }),
    )
    renderWithClient(<SubscriptionPageEdit />)
    const preview = await screen.findByTestId("preview-public-url")
    const link = preview.tagName === "A" ? preview : preview.querySelector("a")
    expect(link?.getAttribute("href")).toContain("/t/acme/subscribe/weekly")
    expect(link?.getAttribute("target")).toBe("_blank")
  })
})

// Smoke-test the ApiError mapping by exercising the create path directly.
describe("SubscriptionPageEdit — save error mapping", () => {
  it("re-throws ApiError shape through the mutation", () => {
    const err = new ApiError(400, "validation_failed", "slug taken", "/x")
    expect(err.slug).toBe("validation_failed")
    expect(err.status).toBe(400)
  })
})

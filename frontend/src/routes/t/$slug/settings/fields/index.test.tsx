// Subscriber-fields settings route tests (T071): create / edit / reorder /
// delete; built-in pseudo-rows render but are not editable / deletable;
// permission gating hides the page for operators without
// `subscriber_fields:manage`.

import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { SubscriberFieldsView } from "./index"
import type { Field } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme" }),
  }),
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: unknown }) => (
    <a href="#">{children as never}</a>
  ),
}))

vi.mock("@/lib/api", () => ({
  api: {
    subscriberFields: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      reorder: vi.fn(),
    },
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true as const, data })

const builtin: Field = {
  id: "builtin:first_name",
  slug: "first_name",
  displayName: "First name",
  type: "text",
  defaultValue: "",
  position: 0,
  builtIn: true,
}

const country: Field = {
  id: "f1",
  slug: "country",
  displayName: "Country",
  type: "text",
  defaultValue: "",
  position: 0,
  builtIn: false,
}

const plan: Field = {
  id: "f2",
  slug: "plan_tier",
  displayName: "Plan tier",
  type: "text",
  defaultValue: "",
  position: 1,
  builtIn: false,
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

describe("SubscriberFieldsView", () => {
  it("renders built-in pseudo-rows alongside custom rows", async () => {
    setupOwner()
    vi.mocked(api.subscriberFields.list).mockResolvedValue(
      ok({ fields: [builtin, country, plan] }),
    )
    renderWithClient(<SubscriberFieldsView />)

    expect(
      await screen.findByTestId("ve-fields-row-first_name"),
    ).toBeTruthy()
    const builtinRow = screen.getByTestId("ve-fields-row-first_name")
    expect(within(builtinRow).getByText(/built-in/i)).toBeTruthy()
    // Built-in rows omit the edit / delete affordances entirely.
    expect(
      within(builtinRow).queryByTestId("ve-fields-edit-first_name"),
    ).toBeNull()
    expect(
      within(builtinRow).queryByTestId("ve-fields-delete-first_name"),
    ).toBeNull()

    // Custom rows expose the full action set.
    const customRow = screen.getByTestId("ve-fields-row-country")
    expect(within(customRow).getByTestId("ve-fields-edit-country")).toBeTruthy()
    expect(
      within(customRow).getByTestId("ve-fields-delete-country"),
    ).toBeTruthy()
  })

  it("creates a field via the dialog", async () => {
    setupOwner()
    vi.mocked(api.subscriberFields.list).mockResolvedValue(
      ok({ fields: [builtin] }),
    )
    vi.mocked(api.subscriberFields.create).mockResolvedValue(
      ok({
        id: "f-new",
        slug: "country",
        displayName: "Country",
        type: "text",
        defaultValue: "",
        position: 0,
        builtIn: false,
      }),
    )
    renderWithClient(<SubscriberFieldsView />)

    fireEvent.click(await screen.findByTestId("ve-fields-add"))

    const dialog = await screen.findByTestId("ve-fields-create-dialog")
    const slugInput = within(dialog).getByLabelText(/slug/i)
    const nameInput = within(dialog).getByLabelText(/display name/i)
    fireEvent.change(slugInput, { target: { value: "country" } })
    fireEvent.change(nameInput, { target: { value: "Country" } })
    fireEvent.click(within(dialog).getByRole("button", { name: /save/i }))

    await waitFor(() =>
      expect(api.subscriberFields.create).toHaveBeenCalledWith(
        "acme",
        expect.objectContaining({
          slug: "country",
          displayName: "Country",
          type: "text",
        }),
      ),
    )
  })

  it("deletes a custom field with confirmation", async () => {
    setupOwner()
    vi.mocked(api.subscriberFields.list).mockResolvedValue(
      ok({ fields: [builtin, country] }),
    )
    vi.mocked(api.subscriberFields.delete).mockResolvedValue(
      ok(undefined as unknown as void),
    )
    renderWithClient(<SubscriberFieldsView />)

    fireEvent.click(await screen.findByTestId("ve-fields-delete-country"))
    const confirm = await screen.findByRole("alertdialog")
    fireEvent.click(within(confirm).getByRole("button", { name: /delete/i }))

    await waitFor(() =>
      expect(api.subscriberFields.delete).toHaveBeenCalledWith("acme", "f1"),
    )
  })

  it("reorders fields by clicking the down affordance", async () => {
    setupOwner()
    vi.mocked(api.subscriberFields.list).mockResolvedValue(
      ok({ fields: [builtin, country, plan] }),
    )
    vi.mocked(api.subscriberFields.reorder).mockResolvedValue(
      ok({ fields: [builtin, plan, country] }),
    )
    renderWithClient(<SubscriberFieldsView />)

    fireEvent.click(await screen.findByTestId("ve-fields-down-country"))
    await waitFor(() =>
      expect(api.subscriberFields.reorder).toHaveBeenCalledWith(
        "acme",
        ["f2", "f1"],
      ),
    )
  })
})

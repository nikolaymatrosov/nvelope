import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, screen } from "@testing-library/react"

import { AccountView } from "./index"
import { renderWithClient } from "@/test/render"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => opts,
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock("@/hooks/use-session", () => ({
  useSession: () => ({
    user: { id: "u1", name: "Ada", email: "ada@example.com", locale: null },
    tenants: [],
  }),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("AccountView", () => {
  it("renders the account page with the language switcher", () => {
    renderWithClient(<AccountView />)

    // getByRole / getByText throw when absent — calling them is the assertion.
    expect(
      screen.getByRole("heading", { name: /account settings/i }),
    ).toBeTruthy()
    expect(screen.getByText(/interface language/i)).toBeTruthy()
  })

  it("marks the active locale in the switcher", () => {
    renderWithClient(<AccountView />)

    // The user has no stored locale, so the default (English) is shown.
    expect(screen.getByRole("combobox").textContent).toContain("English")
  })
})

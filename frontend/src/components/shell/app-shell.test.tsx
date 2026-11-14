import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import { AppShell } from "./app-shell"

const state = vi.hoisted(() => ({ fullscreen: true }))

vi.mock("@tanstack/react-router", () => ({
  useMatches: ({ select }: { select: (m: Array<{ staticData: { fullscreen?: boolean } }>) => boolean }) =>
    select([{ staticData: { fullscreen: state.fullscreen } }]),
}))
vi.mock("@/hooks/use-locale", () => ({ useSyncAccountLocale: vi.fn() }))
vi.mock("./sidebar", () => ({ WorkspaceSidebar: () => <nav data-testid="sidebar" /> }))
vi.mock("./top-bar", () => ({ TopBar: () => <header data-testid="top-bar" /> }))
vi.mock("./suspension-banner", () => ({ SuspensionBanner: () => null }))

afterEach(() => cleanup())

describe("AppShell", () => {
  it("renders only the page for fullscreen routes", () => {
    state.fullscreen = true
    render(
      <AppShell slug="acme" workspaceName="Acme">
        <div data-testid="page" />
      </AppShell>,
    )
    expect(screen.getByTestId("page")).toBeTruthy()
    expect(screen.queryByTestId("sidebar")).toBeNull()
    expect(screen.queryByTestId("top-bar")).toBeNull()
  })

  it("wraps other routes in the sidebar and top bar", () => {
    state.fullscreen = false
    render(
      <AppShell slug="acme" workspaceName="Acme">
        <div data-testid="page" />
      </AppShell>,
    )
    expect(screen.getByTestId("sidebar")).toBeTruthy()
    expect(screen.getByTestId("top-bar")).toBeTruthy()
  })
})

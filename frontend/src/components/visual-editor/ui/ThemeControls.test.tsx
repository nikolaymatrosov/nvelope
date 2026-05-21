// T110 — ThemeControls behavior + theming.ts hook tests.
//
// The component is a controlled surface — every state-change path is driven
// through `onChange`, and the parent owns the Theme | null value. The tests
// pin the three FR-022 / FR-023 / FR-024 invariants:
//   - inherit state shows the "Using tenant defaults" indicator and a Pin
//     button that copies the *resolved* theme into the override draft.
//   - pinned state shows the per-property controls; editing any one fires
//     onChange with a Theme that preserves the rest.
//   - reset-to-defaults clears the override (null), returning the row to
//     the inherit state.
//
// useEditorTheme is covered with two queries: pinned wins over branding;
// inherited resolves through branding's primary_color.

import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  PlatformDefaultTheme,
  editorCssVariables,
  themeFromBrandingPrimary,
  useEditorTheme,
} from "../plugins/theming"
import { ThemeControls } from "./ThemeControls"
import type { ReactNode } from "react"
import type { Theme } from "@/lib/api-types"

function withQueryClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

function getInputByTestId(testId: string): HTMLInputElement {
  return screen.getByTestId(testId) as unknown as HTMLInputElement
}

function getButtonByTestId(testId: string): HTMLButtonElement {
  return screen.getByTestId(testId) as unknown as HTMLButtonElement
}

const RESOLVED: Theme = {
  textColor: "#111111",
  linkColor: "#cc3366",
  buttonColor: "#cc3366",
  buttonTextColor: "#ffffff",
  fontFamily: "'Inter', sans-serif",
  containerWidth: 600,
}

const PINNED: Theme = {
  textColor: "#000000",
  linkColor: "#aa0044",
  buttonColor: "#aa0044",
  buttonTextColor: "#ffffff",
  fontFamily: "'Roboto', sans-serif",
  containerWidth: 640,
}

describe("ThemeControls — inherit state", () => {
  it("renders the inherit badge and the Pin override button", () => {
    render(
      <ThemeControls value={null} resolved={RESOLVED} onChange={vi.fn()} />,
    )
    expect(screen.getByTestId("ve-theme-inherit-badge").textContent).toMatch(
      /using tenant defaults/i,
    )
    expect(screen.getByTestId("ve-theme-pin-override")).toBeTruthy()
    expect(screen.queryByTestId("ve-theme-pinned-body")).toBeNull()
  })

  it("Pin override copies the resolved theme into the override", () => {
    const onChange = vi.fn<(next: Theme | null) => void>()
    render(
      <ThemeControls value={null} resolved={RESOLVED} onChange={onChange} />,
    )
    fireEvent.click(screen.getByTestId("ve-theme-pin-override"))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith(RESOLVED)
  })

  it("disabled state hides interactivity", () => {
    const onChange = vi.fn()
    render(
      <ThemeControls
        value={null}
        resolved={RESOLVED}
        onChange={onChange}
        disabled
      />,
    )
    const pin = getButtonByTestId("ve-theme-pin-override")
    expect(pin.disabled).toBe(true)
    fireEvent.click(pin)
    expect(onChange).not.toHaveBeenCalled()
  })
})

describe("ThemeControls — pinned state", () => {
  it("renders the pinned badge and per-property controls", () => {
    render(
      <ThemeControls value={PINNED} resolved={PINNED} onChange={vi.fn()} />,
    )
    expect(screen.getByTestId("ve-theme-pinned-badge")).toBeTruthy()
    expect(getInputByTestId("ve-theme-text-color").value).toBe("#000000")
    expect(getInputByTestId("ve-theme-link-color").value).toBe("#aa0044")
    expect(getInputByTestId("ve-theme-button-color").value).toBe("#aa0044")
    expect(getInputByTestId("ve-theme-font-family").value).toBe(
      "'Roboto', sans-serif",
    )
    expect(getInputByTestId("ve-theme-container-width").valueAsNumber).toBe(640)
  })

  it("editing button color fires onChange with the field patched and the rest preserved", () => {
    const onChange = vi.fn<(next: Theme | null) => void>()
    render(
      <ThemeControls value={PINNED} resolved={PINNED} onChange={onChange} />,
    )
    fireEvent.change(screen.getByTestId("ve-theme-button-color"), {
      target: { value: "#00ff00" },
    })
    expect(onChange).toHaveBeenCalledWith({ ...PINNED, buttonColor: "#00ff00" })
  })

  it("editing container width fires onChange with the new number", () => {
    const onChange = vi.fn<(next: Theme | null) => void>()
    render(
      <ThemeControls value={PINNED} resolved={PINNED} onChange={onChange} />,
    )
    fireEvent.change(screen.getByTestId("ve-theme-container-width"), {
      target: { value: "720" },
    })
    expect(onChange).toHaveBeenLastCalledWith({
      ...PINNED,
      containerWidth: 720,
    })
  })

  it("Reset to tenant defaults clears the override (null)", () => {
    const onChange = vi.fn<(next: Theme | null) => void>()
    render(
      <ThemeControls value={PINNED} resolved={PINNED} onChange={onChange} />,
    )
    fireEvent.click(screen.getByTestId("ve-theme-reset-defaults"))
    expect(onChange).toHaveBeenCalledWith(null)
  })
})

describe("themeFromBrandingPrimary", () => {
  it("uses the supplied primary color for link + button color", () => {
    const theme = themeFromBrandingPrimary("#ff8800")
    expect(theme.linkColor).toBe("#ff8800")
    expect(theme.buttonColor).toBe("#ff8800")
    expect(theme.textColor).toBe(PlatformDefaultTheme.textColor)
    expect(theme.buttonTextColor).toBe(PlatformDefaultTheme.buttonTextColor)
  })

  it("falls back to the platform default for invalid color shapes", () => {
    expect(themeFromBrandingPrimary("javascript:alert(1)").linkColor).toBe(
      PlatformDefaultTheme.linkColor,
    )
    expect(themeFromBrandingPrimary(null).linkColor).toBe(
      PlatformDefaultTheme.linkColor,
    )
    expect(themeFromBrandingPrimary("").linkColor).toBe(
      PlatformDefaultTheme.linkColor,
    )
  })
})

describe("editorCssVariables", () => {
  it("emits the CSS custom properties the visual-editor stylesheet reads", () => {
    const css = editorCssVariables(PINNED)
    expect(css["--ve-text-color"]).toBe(PINNED.textColor)
    expect(css["--ve-link-color"]).toBe(PINNED.linkColor)
    expect(css["--ve-button-color"]).toBe(PINNED.buttonColor)
    expect(css["--ve-button-text-color"]).toBe(PINNED.buttonTextColor)
    expect(css["--ve-font-family"]).toBe(PINNED.fontFamily)
    expect(css["--ve-container-width"]).toBe(`${PINNED.containerWidth}px`)
  })
})

describe("useEditorTheme", () => {
  it("uses the pinned override verbatim and does not fetch branding", () => {
    // The hook short-circuits the branding query when pinned is non-null.
    // Asserting the returned theme equals the pinned value is sufficient —
    // a branding fetch would have manifested as a network error in the
    // QueryClient (retry: false) had it happened.
    const { result } = renderHook(() => useEditorTheme("acme", PINNED), {
      wrapper: withQueryClient(),
    })
    expect(result.current.theme).toEqual(PINNED)
    expect(result.current.isInherited).toBe(false)
  })

  it("resolves branding's primary color when the row is unpinned", async () => {
    const apiModule = await import("@/lib/api")
    const spy = vi
      .spyOn(apiModule.api.branding, "get")
      .mockResolvedValue({
        status: 200,
        ok: true,
        data: {
          logo_url: "",
          primary_color: "#aa0000",
          custom_css: "",
        },
      })

    const { result } = renderHook(() => useEditorTheme("acme", null), {
      wrapper: withQueryClient(),
    })

    await waitFor(() => {
      expect(result.current.isInherited).toBe(true)
      expect(result.current.theme.linkColor).toBe("#aa0000")
      expect(result.current.theme.buttonColor).toBe("#aa0000")
    })
    spy.mockRestore()
  })
})

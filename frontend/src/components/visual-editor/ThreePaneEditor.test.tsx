// Tests for ThreePaneEditor (T040). jsdom's ResizeObserver is a no-op (see
// src/test/setup.ts), so react-resizable-panels can't perform real layout
// measurement; we therefore assert the shell's structural contracts and the
// responsive (narrow) path rather than pixel-level collapse geometry:
//   - the wide layout renders the canvas between the two side panels with
//     always-available collapse toggles, and toggling does not throw;
//   - on a narrow viewport the side panels become sheet overlays and the canvas
//     stays in the flow.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { ThreePaneEditor } from "./ThreePaneEditor"
import type { VisualDoc } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  // Reset matchMedia to the default (wide) stub from setup.ts.
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
})

const doc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }],
}

function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
}

describe("ThreePaneEditor — wide layout", () => {
  it("renders the canvas with both side panels and collapse toggles", async () => {
    mockMatchMedia(false)
    renderWithClient(<ThreePaneEditor slug="acme" value={doc} onChange={vi.fn()} />)
    expect(screen.getByTestId("ve-three-pane")).toBeTruthy()
    expect(screen.getByTestId("ve-toggle-left")).toBeTruthy()
    expect(screen.getByTestId("ve-toggle-right")).toBeTruthy()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    expect(screen.getByTestId("ve-structure-panel")).toBeTruthy()
    expect(screen.getByTestId("ve-params-panel")).toBeTruthy()
  })

  it("toggling a side panel does not throw", async () => {
    mockMatchMedia(false)
    renderWithClient(<ThreePaneEditor slug="acme" value={doc} onChange={vi.fn()} />)
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    expect(() => {
      fireEvent.click(screen.getByTestId("ve-toggle-left"))
      fireEvent.click(screen.getByTestId("ve-toggle-right"))
    }).not.toThrow()
  })
})

describe("ThreePaneEditor — narrow layout", () => {
  it("presents the side panels as overlays and keeps the canvas in flow", async () => {
    mockMatchMedia(true)
    renderWithClient(<ThreePaneEditor slug="acme" value={doc} onChange={vi.fn()} />)
    await waitFor(() => expect(screen.getByTestId("ve-three-pane")).toBeTruthy())
    expect(screen.getByTestId("ve-three-pane").className).toContain("ve-three-pane--narrow")
    // Overlay triggers replace the in-flow collapse toggles.
    expect(screen.getByTestId("ve-open-left")).toBeTruthy()
    expect(screen.getByTestId("ve-open-right")).toBeTruthy()
    expect(screen.queryByTestId("ve-toggle-left")).toBeNull()
    expect(screen.getByTestId("ve-pane-center")).toBeTruthy()
  })
})

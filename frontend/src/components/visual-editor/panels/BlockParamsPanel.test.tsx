// Tests for BlockParamsPanel (T028). The panel reads the selected block from
// the shared selection model and applies changes via updateSelectedAttrs. We
// drive it with a fabricated BlockSelection so the test focuses on the panel's
// dispatch / control-set / apply logic without a live ProseMirror editor.

import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { BlockParamsPanel } from "./BlockParamsPanel"
import type { BlockSelection } from "../hooks/useBlockSelection"

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

type FakeNode = { type: { name: string }; attrs: Record<string, unknown> }

function selectionFor(node: FakeNode | null): BlockSelection & { updateSelectedAttrs: ReturnType<typeof vi.fn> } {
  return {
    selectedPos: node ? 0 : null,
    // The panel only reads node.type.name and node.attrs.
    selectedNode: node as unknown as BlockSelection["selectedNode"],
    selectBlock: vi.fn(),
    updateSelectedAttrs: vi.fn(),
    clear: vi.fn(),
  }
}

const buttonNode = (attrs: Record<string, unknown> = {}): FakeNode => ({
  type: { name: "button" },
  attrs: { label: "Go", href: "https://example.test/x", ...attrs },
})

describe("BlockParamsPanel", () => {
  it("shows the empty state when nothing is selected", () => {
    render(<BlockParamsPanel selection={selectionFor(null)} />)
    expect(screen.getByTestId("ve-params-empty")).toBeTruthy()
    expect(screen.queryByTestId("ve-params-body")).toBeNull()
  })

  it("shows the button form with its applicable controls when a button is selected", () => {
    render(<BlockParamsPanel selection={selectionFor(buttonNode())} />)
    expect(screen.getByTestId("ve-params-button")).toBeTruthy()
    expect(screen.getByTestId("ve-param-backgroundColor")).toBeTruthy()
    expect(screen.getByTestId("ve-param-borderRadius")).toBeTruthy()
    expect(screen.getByTestId("ve-param-label")).toBeTruthy()
  })

  it("shows only text-applicable controls for a paragraph (no background)", () => {
    const paragraph: FakeNode = { type: { name: "paragraph" }, attrs: {} }
    render(<BlockParamsPanel selection={selectionFor(paragraph)} />)
    expect(screen.getByTestId("ve-params-text")).toBeTruthy()
    expect(screen.getByTestId("ve-param-fontSize")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-backgroundColor")).toBeNull()
  })

  it("applies a style change to the selected block via updateSelectedAttrs", () => {
    const sel = selectionFor(buttonNode())
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.change(screen.getByTestId("ve-param-backgroundColor"), {
      target: { value: "#ff0000" },
    })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({
      style: { backgroundColor: "#ff0000" },
    })
  })

  it("edits a type-specific attr (button label) without touching style", () => {
    const sel = selectionFor(buttonNode())
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.change(screen.getByTestId("ve-param-label"), {
      target: { value: "Read more" },
    })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ label: "Read more" })
  })

  it("merges a new field over the existing style", () => {
    const sel = selectionFor(buttonNode({ style: { backgroundColor: "#111111" } }))
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.change(screen.getByTestId("ve-param-borderRadius"), {
      target: { value: "10" },
    })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({
      style: { backgroundColor: "#111111", borderRadius: 10 },
    })
  })

  it("reset-all clears the block's style", () => {
    const sel = selectionFor(buttonNode({ style: { backgroundColor: "#111111" } }))
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.click(screen.getByTestId("ve-params-reset-all"))
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ style: null })
  })

  it("constrains numeric controls to their email-safe bounds", () => {
    render(<BlockParamsPanel selection={selectionFor(buttonNode())} />)
    const radius = screen.getByTestId("ve-param-borderRadius")
    expect(radius.type).toBe("number")
    expect(radius.min).toBe("0")
    expect(radius.max).toBe("48")
  })
})

// Tests for BlockParamsPanel — Content/Design tabs, the root slot, live-apply
// through the shared selection, and the non-attr actions (replace image,
// change column count). Driven with a fabricated BlockSelection; Radix tabs
// switch on mousedown, so the tests use fireEvent.mouseDown on a trigger.

import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { IMAGEBLOCK_PICK_EVENT } from "../extensions/ImageBlock"
import { setColumnsCount } from "../extensions/Columns"
import { BlockParamsPanel, tabsFor } from "./BlockParamsPanel"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Editor } from "@tiptap/core"

vi.mock("../extensions/Columns", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  setColumnsCount: vi.fn(),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

type FakeNode = { type: { name: string }; attrs: Record<string, unknown> }

function selectionFor(
  node: FakeNode | null,
): BlockSelection & { updateSelectedAttrs: ReturnType<typeof vi.fn> } {
  return {
    selectedPos: node ? 0 : null,
    selectedNode: node as unknown as BlockSelection["selectedNode"],
    selectBlock: vi.fn(),
    updateSelectedAttrs: vi.fn<(attrs: Record<string, unknown>) => void>(),
    clear: vi.fn(),
  }
}

const buttonNode = (attrs: Record<string, unknown> = {}): FakeNode => ({
  type: { name: "button" },
  attrs: { label: "Go", href: "https://example.test/x", ...attrs },
})

const openDesign = () => fireEvent.mouseDown(screen.getByTestId("ve-params-tab-design"))

describe("tabsFor", () => {
  it("maps each block type to its tabs", () => {
    expect(tabsFor(null)).toEqual(["design"])
    expect(tabsFor("button")).toEqual(["content", "design"])
    expect(tabsFor("heading")).toEqual(["content", "design"])
    expect(tabsFor("paragraph")).toEqual(["design"])
    expect(tabsFor("columns")).toEqual(["design"])
    expect(tabsFor("rawHtml")).toEqual(["content"])
    expect(tabsFor("listItem")).toEqual([])
  })
})

describe("BlockParamsPanel", () => {
  it("shows the root slot on the Design tab when nothing is selected", () => {
    render(<BlockParamsPanel selection={selectionFor(null)} root={<div data-testid="root-slot" />} />)
    expect(screen.getByTestId("root-slot")).toBeTruthy()
    expect(screen.getByTestId("ve-params-tab-design")).toBeTruthy()
    expect(screen.queryByTestId("ve-params-tab-content")).toBeNull()
  })

  it("falls back to the empty message without a root slot", () => {
    render(<BlockParamsPanel selection={selectionFor(null)} />)
    expect(screen.getByTestId("ve-params-empty")).toBeTruthy()
  })

  it("opens a button on Content and shows style controls on Design", () => {
    render(<BlockParamsPanel selection={selectionFor(buttonNode())} />)
    expect(screen.getByTestId("ve-param-label")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-backgroundColor")).toBeNull()
    openDesign()
    expect(screen.getByTestId("ve-param-backgroundColor")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-label")).toBeNull()
  })

  it("shows only Design with text controls (no background) for a paragraph", () => {
    render(<BlockParamsPanel selection={selectionFor({ type: { name: "paragraph" }, attrs: {} })} />)
    expect(screen.queryByTestId("ve-params-tab-content")).toBeNull()
    expect(screen.getByTestId("ve-param-fontSize")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-backgroundColor")).toBeNull()
  })

  it("applies a style change to the selected block", () => {
    const sel = selectionFor(buttonNode())
    render(<BlockParamsPanel selection={sel} />)
    openDesign()
    fireEvent.change(screen.getByTestId("ve-param-backgroundColor"), { target: { value: "#ff0000" } })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ style: { backgroundColor: "#ff0000" } })
  })

  it("edits a type-specific attr without touching style", () => {
    const sel = selectionFor(buttonNode())
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.change(screen.getByTestId("ve-param-label"), { target: { value: "Read more" } })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ label: "Read more" })
  })

  it("merges a new field over the existing style", () => {
    const sel = selectionFor(buttonNode({ style: { backgroundColor: "#111111" } }))
    render(<BlockParamsPanel selection={sel} />)
    openDesign()
    fireEvent.click(screen.getByTestId("ve-param-borderRadius-toggle"))
    fireEvent.change(screen.getByTestId("ve-param-borderRadius"), { target: { value: "10" } })
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
    openDesign()
    fireEvent.click(screen.getByTestId("ve-param-borderRadius-toggle"))
    const radius = screen.getByTestId<HTMLInputElement>("ve-param-borderRadius")
    expect(radius.type).toBe("number")
    expect(radius.min).toBe("0")
    expect(radius.max).toBe("48")
  })

  it("asks the canvas to open the media picker for the selected image", () => {
    const dom = document.createElement("div")
    const onPick = vi.fn()
    dom.addEventListener(IMAGEBLOCK_PICK_EVENT, (e) => onPick((e as CustomEvent).detail))
    const editor = { view: { dom } } as unknown as Editor
    const image: FakeNode = { type: { name: "image" }, attrs: { mediaRef: "https://cdn/x.png", alt: "Logo", href: "" } }
    render(<BlockParamsPanel selection={selectionFor(image)} editor={editor} />)
    fireEvent.click(screen.getByTestId("ve-param-replace-image"))
    expect(onPick).toHaveBeenCalledWith({ pos: 0, mediaRef: "https://cdn/x.png", alt: "Logo" })
  })

  it("changes the column count through setColumnsCount", () => {
    const editor = {} as Editor
    const columns: FakeNode = { type: { name: "columns" }, attrs: { count: 2 } }
    render(<BlockParamsPanel selection={selectionFor(columns)} editor={editor} />)
    expect(screen.getByTestId("ve-param-columns-2").getAttribute("aria-pressed")).toBe("true")
    fireEvent.click(screen.getByTestId("ve-param-columns-3"))
    expect(setColumnsCount).toHaveBeenCalledWith(editor, 0, 3)
  })

  it("says there are no properties for a list item", () => {
    render(<BlockParamsPanel selection={selectionFor({ type: { name: "listItem" }, attrs: {} })} />)
    expect(screen.getByTestId("ve-params-none")).toBeTruthy()
  })
})

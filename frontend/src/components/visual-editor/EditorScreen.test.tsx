// EditorScreen with the real TipTap canvas (jsdom can drive it for these
// flows). The router is mocked: only useBlocker is used.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { useBlocker } from "@tanstack/react-router"
import { EditorScreen } from "./EditorScreen"
import type { EditorRow, EditorScreenProps } from "./EditorScreen"
import type { VisualDoc } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"

vi.mock("@tanstack/react-router", () => ({ useBlocker: vi.fn() }))

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

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

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  mockMatchMedia(false)
})

const doc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }],
}

const row: EditorRow = {
  id: "tpl-1",
  name: "Welcome",
  body_doc: doc,
  body_html: "<p>hi</p>",
  theme: null,
  updated_at: "2026-01-01T00:00:00Z",
}

function renderScreen(over: Partial<EditorScreenProps> = {}): EditorScreenProps {
  const props: EditorScreenProps = {
    slug: "acme",
    section: "Templates",
    row,
    editable: true,
    onBack: vi.fn(),
    convert: vi.fn(),
    save: vi.fn().mockResolvedValue({ data: { updatedAt: "2026-01-02T00:00:00Z", warnings: [] } }),
    refetch: vi.fn(),
    onSaved: vi.fn().mockResolvedValue(undefined),
    ...over,
  }
  renderWithClient(<EditorScreen {...props} />)
  return props
}

async function insertDivider() {
  fireEvent.click(screen.getByTestId("ve-add"))
  fireEvent.click(await screen.findByTestId("ve-library-type-divider"))
  fireEvent.click(screen.getByTestId("ve-library-item-divider"))
  await waitFor(() => expect(screen.getByTestId<HTMLButtonElement>("ve-save").disabled).toBe(false))
}

describe("EditorScreen", () => {
  it("renders the top bar, canvas, layers and settings", async () => {
    renderScreen()
    expect(screen.getByTestId("ve-topbar-name").textContent).toBe("Welcome")
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await waitFor(() => expect(screen.getByTestId("ve-structure-panel")).toBeTruthy())
    expect(screen.getByTestId("ve-params-panel")).toBeTruthy()
    expect(screen.getByTestId<HTMLButtonElement>("ve-save").disabled).toBe(true)
  })

  it("inserts a block from the library and saves the new doc", async () => {
    const props = renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await insertDivider()
    fireEvent.click(screen.getByTestId("ve-save"))
    await waitFor(() => expect(props.save).toHaveBeenCalled())
    const sent = vi.mocked(props.save).mock.calls[0][0]
    expect(sent.ifUnmodifiedSince).toBe(row.updated_at)
    expect(sent.bodyDoc.content.some((b) => b.type === "divider")).toBe(true)
  })

  it("saves on Cmd/Ctrl+S when dirty", async () => {
    const props = renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await insertDivider()
    fireEvent.keyDown(window, { key: "s", metaKey: true })
    await waitFor(() => expect(props.save).toHaveBeenCalled())
  })

  it("registers an unsaved-changes guard that follows the dirty flag", async () => {
    renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    const lastOpts = () =>
      vi.mocked(useBlocker).mock.lastCall![0] as unknown as { enableBeforeUnload: () => boolean }
    expect(lastOpts().enableBeforeUnload()).toBe(false)
    await insertDivider()
    expect(lastOpts().enableBeforeUnload()).toBe(true)
  })

  it("Escape in the editor returns the selection to the root", async () => {
    renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await insertDivider()
    fireEvent.keyDown(screen.getByTestId("ve-editor"), { key: "Escape" })
    await waitFor(() => expect(screen.getByTestId("ve-outline-root").classList.contains("is-selected")).toBe(true))
  })

  it("narrows the page on mobile", () => {
    renderScreen()
    fireEvent.click(screen.getByTestId("ve-device-mobile"))
    expect(screen.getByTestId("ve-page").style.width).toBe("375px")
  })

  it("read-only hides editing controls", () => {
    renderScreen({ editable: false })
    expect(screen.getByTestId("ve-readonly-badge")).toBeTruthy()
    expect(screen.queryByTestId("ve-add")).toBeNull()
    expect(screen.queryByTestId("ve-save")).toBeNull()
  })

  it("converts a legacy raw-HTML row on load and starts dirty", async () => {
    const convert = vi.fn().mockResolvedValue({ data: { bodyDoc: doc, warnings: [] } })
    renderScreen({ row: { ...row, body_doc: null, body_html: "<p>legacy</p>" }, convert })
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    expect(convert).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId<HTMLButtonElement>("ve-save").disabled).toBe(false)
  })

  it("offers a way back when conversion fails", async () => {
    const convert = vi.fn().mockRejectedValue(new Error("nope"))
    const props = renderScreen({ row: { ...row, body_doc: null, body_html: "<p>legacy</p>" }, convert })
    expect(await screen.findByTestId("ve-convert-failed")).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Back" }))
    expect(props.onBack).toHaveBeenCalled()
  })

  it("shows the empty state for an empty doc and opens the library from it", async () => {
    renderScreen({ row: { ...row, body_doc: { version: 1, type: "doc", content: [{ type: "paragraph", content: [] }] } } })
    fireEvent.click(await screen.findByTestId("ve-empty-add"))
    expect(await screen.findByTestId("ve-library")).toBeTruthy()
  })

  it("moves the side panel into a sheet on a narrow viewport", async () => {
    mockMatchMedia(true)
    renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-open-panel")).toBeTruthy())
    expect(screen.queryByTestId("ve-params-panel")).toBeNull()
    fireEvent.click(screen.getByTestId("ve-open-panel"))
    expect(await screen.findByTestId("ve-params-panel")).toBeTruthy()
  })
})

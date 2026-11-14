import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { EditorTopBar } from "./EditorTopBar"
import type { ComponentProps } from "react"

afterEach(() => cleanup())

function props(over: Partial<ComponentProps<typeof EditorTopBar>> = {}): ComponentProps<typeof EditorTopBar> {
  return {
    section: "Templates",
    name: "Welcome",
    onBack: vi.fn(),
    editable: true,
    device: "desktop",
    onDeviceChange: vi.fn(),
    canUndo: false,
    canRedo: false,
    onUndo: vi.fn(),
    onRedo: vi.fn(),
    onAdd: vi.fn(),
    onPreview: vi.fn(),
    dirty: false,
    saving: false,
    onSave: vi.fn(),
    ...over,
  }
}

describe("EditorTopBar", () => {
  it("shows breadcrumbs and wires back / add / preview", () => {
    const p = props()
    render(<EditorTopBar {...p} />)
    expect(screen.getByTestId("ve-topbar-name").textContent).toBe("Welcome")
    expect(screen.getByText("Templates")).toBeTruthy()
    fireEvent.click(screen.getByTestId("ve-back"))
    fireEvent.click(screen.getByTestId("ve-add"))
    fireEvent.click(screen.getByTestId("ve-preview"))
    expect(p.onBack).toHaveBeenCalled()
    expect(p.onAdd).toHaveBeenCalled()
    expect(p.onPreview).toHaveBeenCalled()
  })

  it("disables undo/redo per editor history", () => {
    render(<EditorTopBar {...props({ canUndo: true })} />)
    expect(screen.getByTestId<HTMLButtonElement>("ve-undo").disabled).toBe(false)
    expect(screen.getByTestId<HTMLButtonElement>("ve-redo").disabled).toBe(true)
  })

  it("marks the current device and switches it", () => {
    const p = props()
    render(<EditorTopBar {...p} />)
    expect(screen.getByTestId("ve-device-desktop").getAttribute("aria-pressed")).toBe("true")
    fireEvent.click(screen.getByTestId("ve-device-mobile"))
    expect(p.onDeviceChange).toHaveBeenCalledWith("mobile")
  })

  it("enables Save only when dirty and labels its state", () => {
    const { rerender } = render(<EditorTopBar {...props()} />)
    const save = () => screen.getByTestId<HTMLButtonElement>("ve-save")
    expect(save().disabled).toBe(true)
    expect(save().textContent).toBe("Saved")
    rerender(<EditorTopBar {...props({ dirty: true })} />)
    expect(save().disabled).toBe(false)
    expect(save().textContent).toBe("Save")
    rerender(<EditorTopBar {...props({ dirty: true, saving: true })} />)
    expect(save().disabled).toBe(true)
    expect(save().textContent).toBe("Saving…")
  })

  it("read-only hides editing controls and shows a badge", () => {
    render(<EditorTopBar {...props({ editable: false })} />)
    expect(screen.getByTestId("ve-readonly-badge")).toBeTruthy()
    expect(screen.queryByTestId("ve-add")).toBeNull()
    expect(screen.queryByTestId("ve-undo")).toBeNull()
    expect(screen.queryByTestId("ve-save")).toBeNull()
  })

  it("shows the panel button only when given (narrow viewport)", () => {
    const { rerender } = render(<EditorTopBar {...props()} />)
    expect(screen.queryByTestId("ve-open-panel")).toBeNull()
    rerender(<EditorTopBar {...props({ onOpenPanel: vi.fn() })} />)
    expect(screen.getByTestId("ve-open-panel")).toBeTruthy()
  })
})

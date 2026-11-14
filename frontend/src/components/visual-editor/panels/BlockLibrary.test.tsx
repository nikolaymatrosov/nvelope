import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { BlockLibrary } from "./BlockLibrary"

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("BlockLibrary", () => {
  it("renders nothing while closed", () => {
    render(<BlockLibrary open={false} onOpenChange={vi.fn()} onInsert={vi.fn()} />)
    expect(screen.queryByTestId("ve-library")).toBeNull()
  })

  it("lists block types and shows Text variants first", () => {
    render(<BlockLibrary open onOpenChange={vi.fn()} onInsert={vi.fn()} />)
    expect(screen.getByText("Block library")).toBeTruthy()
    expect(screen.getByTestId("ve-library-type-heading")).toBeTruthy()
    expect(screen.getByTestId("ve-library-item-paragraph")).toBeTruthy()
    expect(screen.queryByTestId("ve-library-item-heading-1")).toBeNull()
  })

  it("switches type and inserts the clicked variant", () => {
    const onInsert = vi.fn()
    render(<BlockLibrary open onOpenChange={vi.fn()} onInsert={onInsert} />)
    fireEvent.click(screen.getByTestId("ve-library-type-heading"))
    expect(screen.getByTestId("ve-library-item-heading-3")).toBeTruthy()
    fireEvent.click(screen.getByTestId("ve-library-item-heading-2"))
    expect(onInsert).toHaveBeenCalledWith(expect.objectContaining({ key: "heading-2", type: "heading" }))
  })
})

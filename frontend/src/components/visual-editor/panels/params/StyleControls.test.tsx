import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { StyleControls } from "./StyleControls"

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("StyleControls", () => {
  it("renders only the sections that have fields", () => {
    render(<StyleControls value={{}} fields={["color", "padding"]} onChange={vi.fn()} />)
    expect(screen.getByTestId("ve-style-section-typography")).toBeTruthy()
    expect(screen.getByTestId("ve-style-section-spacing")).toBeTruthy()
    expect(screen.queryByTestId("ve-style-section-background")).toBeNull()
    expect(screen.queryByTestId("ve-style-section-border")).toBeNull()
  })

  it("hides border fields behind an off switch until it is turned on", () => {
    render(<StyleControls value={{}} fields={["border"]} onChange={vi.fn()} />)
    const toggle = screen.getByTestId("ve-param-border-toggle") as unknown as HTMLInputElement
    expect(toggle.checked).toBe(false)
    expect(screen.queryByTestId("ve-param-borderWidth")).toBeNull()
    fireEvent.click(toggle)
    expect(screen.getByTestId("ve-param-borderWidth")).toBeTruthy()
  })

  it("turning the border switch off clears all border fields", () => {
    const onChange = vi.fn()
    render(
      <StyleControls
        value={{ borderWidth: 2, borderStyle: "solid", borderColor: "#000000" }}
        fields={["border"]}
        onChange={onChange}
      />,
    )
    const toggle = screen.getByTestId("ve-param-border-toggle") as unknown as HTMLInputElement
    expect(toggle.checked).toBe(true)
    fireEvent.click(toggle)
    expect(onChange).toHaveBeenCalledWith({
      borderWidth: undefined,
      borderStyle: undefined,
      borderColor: undefined,
    })
  })

  it("puts corner radius behind its own switch", () => {
    const onChange = vi.fn()
    render(<StyleControls value={{ borderRadius: 12 }} fields={["borderRadius"]} onChange={onChange} />)
    const toggle = screen.getByTestId("ve-param-borderRadius-toggle") as unknown as HTMLInputElement
    expect(toggle.checked).toBe(true)
    expect((screen.getByTestId("ve-param-borderRadius") as unknown as HTMLInputElement).value).toBe("12")
    fireEvent.click(toggle)
    expect(onChange).toHaveBeenCalledWith({ borderRadius: undefined })
  })

  it("lays padding out left, top, bottom, right", () => {
    const { container } = render(<StyleControls value={{}} fields={["padding"]} onChange={vi.fn()} />)
    const ids = Array.from(container.querySelectorAll("[data-testid^='ve-param-padding']")).map((el) =>
      el.getAttribute("data-testid"),
    )
    expect(ids).toEqual([
      "ve-param-paddingLeft",
      "ve-param-paddingTop",
      "ve-param-paddingBottom",
      "ve-param-paddingRight",
    ])
  })
})

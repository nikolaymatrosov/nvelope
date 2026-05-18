import { afterEach, describe, expect, it } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import { MetricTile } from "./metric-tile"

afterEach(cleanup)

describe("MetricTile", () => {
  it("renders a label and a formatted count", () => {
    render(<MetricTile label="Sent" value={12345} />)
    expect(screen.getByText("Sent")).toBeDefined()
    expect(screen.getByText("12,345")).toBeDefined()
  })

  it("renders the optional rate line when supplied", () => {
    render(<MetricTile label="Opened" value={10} rate="50%" />)
    expect(screen.getByText("50%")).toBeDefined()
  })

  it("omits the rate line when not supplied", () => {
    const { container } = render(<MetricTile label="Bounced" value={3} />)
    expect(container.querySelectorAll("p").length).toBe(2)
  })
})

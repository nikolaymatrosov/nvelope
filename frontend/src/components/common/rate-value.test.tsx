import { afterEach, describe, expect, it } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import { RateValue } from "./rate-value"

afterEach(cleanup)

describe("RateValue", () => {
  it("renders a fraction as a percentage", () => {
    render(<RateValue value={0.1234} />)
    expect(screen.getByText("12.3%")).toBeDefined()
  })

  it("renders a zero rate as 0%", () => {
    render(<RateValue value={0} />)
    expect(screen.getByText("0%")).toBeDefined()
  })

  it("drops trailing zeros for a whole percentage", () => {
    render(<RateValue value={0.5} />)
    expect(screen.getByText("50%")).toBeDefined()
  })

  it("falls back to 0% for a non-finite value", () => {
    render(<RateValue value={Number.NaN} />)
    expect(screen.getByText("0%")).toBeDefined()
  })
})

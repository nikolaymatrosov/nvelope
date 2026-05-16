import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { Button } from "./button"

describe("Button", () => {
  it("renders its children", () => {
    render(<Button>nvelope</Button>)
    expect(screen.getByRole("button", { name: "nvelope" })).toBeDefined()
  })

  it("applies the requested variant", () => {
    render(<Button variant="destructive">delete</Button>)
    expect(
      screen.getByRole("button", { name: "delete" }).dataset.variant,
    ).toBe("destructive")
  })
})

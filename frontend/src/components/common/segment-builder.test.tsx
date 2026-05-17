import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { SegmentBuilder, emptyGroup } from "./segment-builder"
import type { Node } from "@/lib/api-types"

afterEach(cleanup)

describe("SegmentBuilder", () => {
  it("adds a field condition to the group", () => {
    const onChange = vi.fn()
    render(<SegmentBuilder value={emptyGroup()} onChange={onChange} />)
    fireEvent.click(screen.getByRole("button", { name: /condition/i }))

    expect(onChange).toHaveBeenCalledTimes(1)
    const next = onChange.mock.calls[0][0] as Node
    expect(next.Children).toHaveLength(1)
    expect(next.Children?.[0].Field).toEqual({
      Field: "email",
      Op: "eq",
      Value: "",
    })
  })

  it("renders existing leaf conditions", () => {
    const value: Node = {
      Conj: "or",
      Children: [{ Field: { Field: "name", Op: "contains", Value: "ann" } }],
    }
    render(<SegmentBuilder value={value} onChange={vi.fn()} />)
    expect(screen.getByDisplayValue("ann")).toBeDefined()
  })

  it("removes a condition", () => {
    const onChange = vi.fn()
    const value: Node = {
      Conj: "and",
      Children: [{ Field: { Field: "email", Op: "eq", Value: "a@b.com" } }],
    }
    render(<SegmentBuilder value={value} onChange={onChange} />)
    fireEvent.click(screen.getByRole("button", { name: /remove condition/i }))

    const next = onChange.mock.calls[0][0] as Node
    expect(next.Children).toHaveLength(0)
  })
})

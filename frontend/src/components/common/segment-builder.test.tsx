import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { SegmentBuilder, emptyGroup } from "./segment-builder"
import type { SegmentNode } from "@/lib/api-types"

afterEach(cleanup)

describe("SegmentBuilder", () => {
  it("adds a field condition to the group", () => {
    const onChange = vi.fn()
    render(<SegmentBuilder value={emptyGroup()} onChange={onChange} />)
    fireEvent.click(screen.getByRole("button", { name: /condition/i }))

    expect(onChange).toHaveBeenCalledTimes(1)
    const next = onChange.mock.calls[0][0] as SegmentNode
    expect(next.children).toHaveLength(1)
    expect(next.children?.[0].field).toEqual({
      field: "email",
      op: "eq",
      value: "",
    })
  })

  it("renders existing leaf conditions", () => {
    const value: SegmentNode = {
      conj: "or",
      children: [{ field: { field: "name", op: "contains", value: "ann" } }],
    }
    render(<SegmentBuilder value={value} onChange={vi.fn()} />)
    expect(screen.getByDisplayValue("ann")).toBeDefined()
  })

  it("removes a condition", () => {
    const onChange = vi.fn()
    const value: SegmentNode = {
      conj: "and",
      children: [{ field: { field: "email", op: "eq", value: "a@b.com" } }],
    }
    render(<SegmentBuilder value={value} onChange={onChange} />)
    fireEvent.click(screen.getByRole("button", { name: /remove condition/i }))

    const next = onChange.mock.calls[0][0] as SegmentNode
    expect(next.children).toHaveLength(0)
  })
})

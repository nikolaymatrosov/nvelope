import { useState } from "react"
import { expect, fn, userEvent } from "storybook/test"
import { SegmentBuilder, emptyGroup } from "./segment-builder"
import type { Node } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

// Stateful harness so added/removed conditions round-trip through the recursive
// onChange tree the way the audience form drives it.
function Harness({
  initial,
  onChange,
}: {
  initial: Node
  onChange?: (next: Node) => void
}) {
  const [value, setValue] = useState<Node>(initial)
  return (
    <SegmentBuilder
      value={value}
      onChange={(next) => {
        setValue(next)
        onChange?.(next)
      }}
    />
  )
}

const populated: Node = {
  Conj: "or",
  Children: [
    { Field: { Field: "name", Op: "contains", Value: "ann" } },
    {
      Conj: "and",
      Children: [
        { Field: { Field: "state", Op: "eq", Value: "subscribed" } },
        { Attr: { Key: "plan", Op: "eq", Value: "pro" } },
      ],
    },
  ],
}

const meta = {
  component: SegmentBuilder,
  tags: ["ai-generated"],
  args: { value: emptyGroup(), onChange: fn() },
} satisfies Meta<typeof SegmentBuilder>

export default meta
type Story = StoryObj<typeof meta>

// An empty top-level group with just the add-condition / add-group affordances.
export const EmptyGroup: Story = {
  render: () => <Harness initial={emptyGroup()} />,
}

// A nested tree: an "any" group containing a leaf and an "all" subgroup.
export const NestedGroups: Story = {
  render: () => <Harness initial={populated} />,
  play: async ({ canvas }) => {
    // The existing leaf's value is rendered in an input.
    await expect(canvas.getByDisplayValue("ann")).toBeVisible()
    await expect(canvas.getByDisplayValue("pro")).toBeVisible()
  },
}

// Clicking "Add condition" appends a field leaf to the tree (onChange fires).
export const AddCondition: Story = {
  render: (args) => <Harness initial={emptyGroup()} onChange={args.onChange} />,
  play: async ({ canvas, args }) => {
    await userEvent.click(
      canvas.getByRole("button", { name: /^condition$/i }),
    )
    await expect(args.onChange).toHaveBeenCalled()
    const next = args.onChange.mock.calls[0][0]
    await expect(next.Children).toHaveLength(1)
    await expect(next.Children?.[0].Field).toEqual({
      Field: "email",
      Op: "eq",
      Value: "",
    })
  },
}

// Removing the only condition empties the group's children.
export const RemoveCondition: Story = {
  render: (args) => (
    <Harness
      initial={{
        Conj: "and",
        Children: [{ Field: { Field: "email", Op: "eq", Value: "a@b.com" } }],
      }}
      onChange={args.onChange}
    />
  ),
  play: async ({ canvas, args }) => {
    await userEvent.click(
      canvas.getByRole("button", { name: /remove condition/i }),
    )
    await expect(args.onChange).toHaveBeenCalled()
    const next = args.onChange.mock.calls[0][0]
    await expect(next.Children).toHaveLength(0)
  },
}

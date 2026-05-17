import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { JsonAttributeEditor } from "./json-attribute-editor"

afterEach(cleanup)

describe("JsonAttributeEditor", () => {
  it("reports invalid JSON and blocks validity", () => {
    const onChange = vi.fn()
    const onValidityChange = vi.fn()
    render(
      <JsonAttributeEditor
        value={{}}
        onChange={onChange}
        onValidityChange={onValidityChange}
      />,
    )
    fireEvent.change(screen.getByLabelText(/custom attributes/i), {
      target: { value: "{ not json" },
    })
    expect(screen.getByText(/not valid json/i)).toBeDefined()
    expect(onValidityChange).toHaveBeenLastCalledWith(false)
  })

  it("rejects a non-object JSON value", () => {
    const onValidityChange = vi.fn()
    render(
      <JsonAttributeEditor
        value={{}}
        onChange={vi.fn()}
        onValidityChange={onValidityChange}
      />,
    )
    fireEvent.change(screen.getByLabelText(/custom attributes/i), {
      target: { value: "[1, 2, 3]" },
    })
    expect(screen.getByText(/must be a json object/i)).toBeDefined()
    expect(onValidityChange).toHaveBeenLastCalledWith(false)
  })

  it("emits the parsed object for valid JSON", () => {
    const onChange = vi.fn()
    const onValidityChange = vi.fn()
    render(
      <JsonAttributeEditor
        value={{}}
        onChange={onChange}
        onValidityChange={onValidityChange}
      />,
    )
    fireEvent.change(screen.getByLabelText(/custom attributes/i), {
      target: { value: '{ "plan": "pro" }' },
    })
    expect(onChange).toHaveBeenLastCalledWith({ plan: "pro" })
    expect(onValidityChange).toHaveBeenLastCalledWith(true)
  })
})

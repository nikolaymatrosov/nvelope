// Custom-attribute editor (FR-013, research.md Decision 8). Holds the attribute
// object as formatted JSON text, validates with JSON.parse on every change, and
// reports validity so the parent form can block save on invalid structure.

import { useState } from "react"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

type JsonAttributeEditorProps = {
  value: Record<string, unknown>
  onChange: (next: Record<string, unknown>) => void
  onValidityChange?: (valid: boolean) => void
  label?: string
}

function pretty(value: Record<string, unknown>): string {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return "{}"
  }
}

export function JsonAttributeEditor({
  value,
  onChange,
  onValidityChange,
  label = "Custom attributes",
}: JsonAttributeEditorProps) {
  const [text, setText] = useState(() => pretty(value))
  const [error, setError] = useState<string | null>(null)

  function handleChange(next: string) {
    setText(next)
    const trimmed = next.trim()
    if (trimmed === "") {
      setError(null)
      onValidityChange?.(true)
      onChange({})
      return
    }
    let parsed: unknown
    try {
      parsed = JSON.parse(trimmed)
    } catch {
      setError("Not valid JSON.")
      onValidityChange?.(false)
      return
    }
    if (
      typeof parsed !== "object" ||
      parsed === null ||
      Array.isArray(parsed)
    ) {
      setError("Attributes must be a JSON object, e.g. { \"plan\": \"pro\" }.")
      onValidityChange?.(false)
      return
    }
    setError(null)
    onValidityChange?.(true)
    onChange(parsed as Record<string, unknown>)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor="json-attributes">{label}</Label>
      <Textarea
        id="json-attributes"
        className={cn("font-mono text-xs", error && "border-destructive")}
        rows={8}
        spellCheck={false}
        aria-invalid={error ? true : undefined}
        value={text}
        onChange={(e) => handleChange(e.target.value)}
      />
      {error ? (
        <p className="text-xs text-destructive" role="alert">
          {error}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">
          A JSON object of custom fields stored with this subscriber.
        </p>
      )}
    </div>
  )
}

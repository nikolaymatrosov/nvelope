// Field chrome and validators for TanStack Form (research.md Decision 7).
// `FormField` is the presentational wrapper — label, inline error, busy state;
// forms bind it to a `form.Field` render prop. `rules` are plain validator
// functions compatible with TanStack Form's `validators` option.

import { useId } from "react"
import type { ReactNode } from "react"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

type FormFieldProps = {
  label: string
  error?: string
  hint?: string
  required?: boolean
  children?: ReactNode
} & Omit<React.ComponentProps<"input">, "id">

// FormField renders a standard <Input> unless `children` is supplied, in which
// case it wraps an arbitrary control (select, textarea) with the same label /
// error chrome.
export function FormField({
  label,
  error,
  hint,
  required,
  children,
  className,
  ...inputProps
}: FormFieldProps) {
  const id = useId()
  return (
    <div className="flex flex-col gap-1.5" data-invalid={error ? true : undefined}>
      <Label htmlFor={id}>
        {label}
        {required && <span className="text-destructive"> *</span>}
      </Label>
      {children ?? (
        <Input
          id={id}
          aria-invalid={error ? true : undefined}
          className={cn(className)}
          {...inputProps}
        />
      )}
      {hint && !error && (
        <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {error && (
        <p className="text-xs text-destructive" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

// ── Validators ───────────────────────────────────────────────────────────────
// A validator returns an error message string, or undefined when the value is
// valid — the shape TanStack Form's `validators` callbacks expect.

export type Validator = (value: string) => string | undefined

export const rules = {
  required:
    (message = "This field is required."): Validator =>
    (v) =>
      v.trim() ? undefined : message,
  email:
    (message = "Enter a valid email address."): Validator =>
    (v) =>
      /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim()) ? undefined : message,
  minLength:
    (n: number, message?: string): Validator =>
    (v) =>
      v.length >= n
        ? undefined
        : (message ?? `Must be at least ${n} characters.`),
  slug:
    (message = "Use lowercase letters, numbers, and hyphens only."): Validator =>
    (v) =>
      /^[a-z0-9-]+$/.test(v.trim()) ? undefined : message,
}

// Compose several validators into one TanStack Form field validator. Returns
// the first failing message, or undefined when every rule passes.
export function compose(...validators: Array<Validator>) {
  return ({ value }: { value: string }): string | undefined => {
    for (const validate of validators) {
      const message = validate(value)
      if (message) return message
    }
    return undefined
  }
}

// Extract a single display message from a TanStack Form field's error list.
export function fieldError(errors: ReadonlyArray<unknown>): string | undefined {
  for (const e of errors) {
    if (typeof e === "string" && e) return e
    if (e && typeof e === "object" && "message" in e) {
      const m = (e as { message?: unknown }).message
      if (typeof m === "string" && m) return m
    }
  }
  return undefined
}

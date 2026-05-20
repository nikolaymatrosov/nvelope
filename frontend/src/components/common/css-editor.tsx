// Textarea wrapper for the tenant's custom CSS. Sanitization is enforced by
// the backend; the editor surfaces the configured size limit, blocks save when
// the input exceeds it, and (when a sanitized copy is supplied) shows the
// server-returned, sanitized result as a read-only preview block.

import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

type CssEditorProps = {
  value: string
  onChange: (value: string) => void
  limitBytes: number
  // The sanitized CSS returned by the server on the most recent save, shown
  // read-only beneath the input. `null` or empty hides the preview block.
  sanitized?: string | null
  label?: string
  disabled?: boolean
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

export function CssEditor({
  value,
  onChange,
  limitBytes,
  sanitized,
  label = "Custom CSS",
  disabled,
}: CssEditorProps) {
  const used = byteLength(value)
  const overLimit = used > limitBytes
  const showPreview = sanitized !== undefined && sanitized !== null && sanitized !== ""
  return (
    <div className="flex flex-col gap-2" data-testid="css-editor">
      <Label htmlFor="css-editor-input">{label}</Label>
      <p className="text-xs text-muted-foreground">
        CSS is sanitized server-side before it is applied to your public pages.
        Limit: {limitBytes.toLocaleString()} bytes.
      </p>
      <Textarea
        id="css-editor-input"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        rows={10}
        className={cn(
          "font-mono text-sm",
          overLimit && "border-destructive focus-visible:ring-destructive",
        )}
        data-invalid={overLimit ? true : undefined}
        data-testid="css-editor-input"
        aria-invalid={overLimit ? true : undefined}
      />
      <div className="flex items-center justify-between text-xs">
        <span
          className={cn(
            "tabular-nums",
            overLimit ? "text-destructive" : "text-muted-foreground",
          )}
          data-testid="css-editor-counter"
        >
          {used.toLocaleString()} / {limitBytes.toLocaleString()} bytes
        </span>
        {overLimit && (
          <span className="text-destructive" role="alert">
            Reduce the CSS below the limit before saving.
          </span>
        )}
      </div>
      {showPreview && (
        <div
          className="rounded-md border bg-muted/30 p-3"
          data-testid="css-editor-sanitized"
        >
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            Sanitized CSS applied to your public pages
          </p>
          <pre className="overflow-x-auto whitespace-pre-wrap break-all text-xs font-mono">
            {sanitized}
          </pre>
        </div>
      )}
    </div>
  )
}

export function isCssOverLimit(value: string, limitBytes: number): boolean {
  return byteLength(value) > limitBytes
}

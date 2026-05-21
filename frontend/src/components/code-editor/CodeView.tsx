// CodeView — thin controlled wrapper around @uiw/react-codemirror with the
// HTML language extension preconfigured. Used in two places per the
// 014-visual-email-editor plan: (a) the full-page code editor swap-in for
// legacy raw-HTML campaigns/templates and operators who opt out of the
// visual editor (FR-029); (b) the modal hosted by the visual editor's
// RawHTML block "Edit HTML" affordance (FR-027).
//
// Keep this component dumb: the parent owns the value and decides what to
// do with the edits. The wrapper only exposes onChange + light styling so
// the surface stays portable between the two hosts.

import { useMemo } from "react"
import CodeMirror from "@uiw/react-codemirror"
import { html } from "@codemirror/lang-html"
import type { Extension } from "@uiw/react-codemirror"

type Props = {
  value: string
  onChange: (next: string) => void
  // Editable defaults to true; set false for a read-only preview of the
  // server-rendered HTML (e.g. when peeking at a sanitization warning).
  editable?: boolean
  // Optional className passed through to the CodeMirror container so the
  // parent can size the editor. Defaults to a full-height container.
  className?: string
  // Optional placeholder displayed when the value is empty.
  placeholder?: string
  // Optional aria-label / test id — handy when several CodeView
  // instances appear on the same screen.
  ariaLabel?: string
  testId?: string
}

export function CodeView({
  value,
  onChange,
  editable = true,
  className,
  placeholder,
  ariaLabel,
  testId,
}: Props) {
  const extensions = useMemo<Array<Extension>>(
    () => [html({ matchClosingTags: true, autoCloseTags: true })],
    [],
  )

  return (
    <div
      className={className ?? "cv-root"}
      data-testid={testId ?? "code-view"}
      aria-label={ariaLabel}
    >
      <CodeMirror
        value={value}
        editable={editable}
        onChange={(next: string) => onChange(next)}
        extensions={extensions}
        placeholder={placeholder}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: true,
          foldGutter: true,
          autocompletion: true,
          bracketMatching: true,
        }}
        theme="light"
      />
    </div>
  )
}

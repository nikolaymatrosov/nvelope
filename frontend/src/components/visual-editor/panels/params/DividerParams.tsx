// Parameters for the divider block — feature 017, T025. The border group maps
// to the rule line (color / width / style); padding becomes the spacing above
// and below.

import { StyleControls } from "./StyleControls"
import type { ParamFormProps, StyleField } from "./StyleControls"

const DIVIDER_FIELDS: ReadonlyArray<StyleField> = ["border", "padding"]

export function DividerParams({ style, onStyleChange, disabled }: ParamFormProps) {
  return (
    <div className="ve-params-form" data-testid="ve-params-divider">
      <StyleControls value={style} fields={DIVIDER_FIELDS} onChange={onStyleChange} disabled={disabled} />
    </div>
  )
}

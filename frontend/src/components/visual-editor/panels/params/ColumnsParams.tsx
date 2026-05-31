// Parameters for the columns container (and a single column) — feature 017,
// T025. Container-level styling (background, padding, corner radius, border)
// applies to the row/cell. The column count is shown read-only here; columns
// are added/removed structurally on the canvas.

import { useTranslation } from "react-i18next"
import { StyleControls } from "./StyleControls"
import type { ParamFormProps, StyleField } from "./StyleControls"

const CONTAINER_FIELDS: ReadonlyArray<StyleField> = [
  "backgroundColor",
  "padding",
  "borderRadius",
  "border",
]

export function ColumnsParams({ blockType, attrs, style, onStyleChange, disabled }: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-params-form" data-testid="ve-params-columns">
      {blockType === "columns" && attrs.count != null && (
        <p className="ve-style-controls__readonly" data-testid="ve-param-column-count">
          {t("structure.block.columns").replace("{{count}}", String(attrs.count))}
        </p>
      )}
      <StyleControls value={style} fields={CONTAINER_FIELDS} onChange={onStyleChange} disabled={disabled} />
    </div>
  )
}

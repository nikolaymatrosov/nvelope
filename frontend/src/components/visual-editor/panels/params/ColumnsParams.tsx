// Parameters for the columns container (and a single column) — feature 017,
// T025. Container-level styling (background, padding, corner radius, border)
// applies to the row/cell. The column count is changed here via setColumnsCount.

import { useTranslation } from "react-i18next"
import { setColumnsCount } from "../../extensions/Columns"
import { StyleControls } from "./StyleControls"
import type { ParamFormProps, StyleField } from "./StyleControls"

const CONTAINER_FIELDS: ReadonlyArray<StyleField> = [
  "backgroundColor",
  "padding",
  "borderRadius",
  "border",
]

const COUNTS = [2, 3, 4] as const

export function ColumnsParams({
  blockType,
  attrs,
  style,
  onStyleChange,
  disabled,
  editor = null,
  pos = 0,
}: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-params-form" data-testid="ve-params-columns">
      {blockType === "columns" && (
        <div
          className="ve-style-controls__row"
          role="group"
          aria-label={t("params.fields.columnCount")}
          data-testid="ve-param-column-count"
        >
          <span className="ve-style-controls__label">{t("params.fields.columnCount")}</span>
          <span className="ve-style-controls__align">
            {COUNTS.map((n) => (
              <button
                key={n}
                type="button"
                data-testid={`ve-param-columns-${n}`}
                aria-pressed={attrs.count === n}
                className={attrs.count === n ? "is-active" : ""}
                disabled={disabled || editor === null}
                onClick={() => {
                  if (editor) setColumnsCount(editor, pos, n)
                }}
              >
                {n}
              </button>
            ))}
          </span>
        </div>
      )}
      <StyleControls value={style} fields={CONTAINER_FIELDS} onChange={onStyleChange} disabled={disabled} />
    </div>
  )
}

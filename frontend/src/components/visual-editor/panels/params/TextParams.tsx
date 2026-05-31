// Parameters for text blocks (paragraph, heading, blockquote, lists) — feature
// 017, T025. Exposes typography, alignment, and spacing; heading additionally
// exposes its level. The applicable field set follows the data-model matrix.

import { useTranslation } from "react-i18next"
import { StyleControls } from "./StyleControls"
import type { ParamFormProps, StyleField } from "./StyleControls"

const TEXT_FIELDS: ReadonlyArray<StyleField> = [
  "color",
  "fontFamily",
  "fontSize",
  "fontWeight",
  "lineHeight",
  "textAlign",
  "padding",
]

export function TextParams({
  blockType,
  attrs,
  style,
  onStyleChange,
  onAttrsChange,
  disabled,
}: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-params-form" data-testid="ve-params-text">
      {blockType === "heading" && (
        <label className="ve-style-controls__row">
          <span className="ve-style-controls__label">{t("params.fields.level")}</span>
          <select
            className="ve-style-controls__field"
            data-testid="ve-param-level"
            value={String((attrs.level as number | undefined) ?? 1)}
            onChange={(e) => onAttrsChange({ level: Number(e.target.value) })}
            disabled={disabled}
            aria-label={t("params.fields.level")}
          >
            {[1, 2, 3].map((lvl) => (
              <option key={lvl} value={lvl}>
                {`H${lvl}`}
              </option>
            ))}
          </select>
        </label>
      )}
      <StyleControls value={style} fields={TEXT_FIELDS} onChange={onStyleChange} disabled={disabled} />
    </div>
  )
}

// Parameters for the button block — feature 017, T025. Exposes the label and
// link plus background/text color, corner radius, padding, border, and font.

import { useTranslation } from "react-i18next"
import { StyleControls, TextInput } from "./StyleControls"
import type { ParamFormProps, StyleField } from "./StyleControls"

const BUTTON_FIELDS: ReadonlyArray<StyleField> = [
  "backgroundColor",
  "color",
  "fontFamily",
  "fontSize",
  "fontWeight",
  "borderRadius",
  "padding",
  "border",
]

export function ButtonParams({ attrs, style, onStyleChange, onAttrsChange, disabled }: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-params-form" data-testid="ve-params-button">
      <TextInput
        label={t("params.fields.label")}
        testId="ve-param-label"
        value={(attrs.label as string | undefined) ?? ""}
        onChange={(v) => onAttrsChange({ label: v })}
        disabled={disabled}
      />
      <TextInput
        label={t("params.fields.href")}
        testId="ve-param-href"
        value={(attrs.href as string | undefined) ?? ""}
        onChange={(v) => onAttrsChange({ href: v })}
        disabled={disabled}
      />
      <StyleControls value={style} fields={BUTTON_FIELDS} onChange={onStyleChange} disabled={disabled} />
    </div>
  )
}

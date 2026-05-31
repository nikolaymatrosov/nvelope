// Parameters for the image block — feature 017, T025. Exposes alt text and the
// optional link plus corner radius, border, and alignment. (Background and font
// don't apply to an image.)

import { useTranslation } from "react-i18next"
import { StyleControls, TextInput } from "./StyleControls"
import type { ParamFormProps, StyleField } from "./StyleControls"

const IMAGE_FIELDS: ReadonlyArray<StyleField> = ["textAlign", "borderRadius", "border"]

export function ImageParams({ attrs, style, onStyleChange, onAttrsChange, disabled }: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-params-form" data-testid="ve-params-image">
      <TextInput
        label={t("params.fields.alt")}
        testId="ve-param-alt"
        value={(attrs.alt as string | undefined) ?? ""}
        onChange={(v) => onAttrsChange({ alt: v })}
        disabled={disabled}
      />
      <TextInput
        label={t("params.fields.href")}
        testId="ve-param-image-href"
        value={(attrs.href as string | undefined) ?? ""}
        onChange={(v) => onAttrsChange({ href: v })}
        disabled={disabled}
      />
      <StyleControls value={style} fields={IMAGE_FIELDS} onChange={onStyleChange} disabled={disabled} />
    </div>
  )
}

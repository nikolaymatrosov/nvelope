// Parameters for the image block — feature 017, T025. Exposes alt text and the
// optional link plus corner radius, border, and alignment. (Background and font
// don't apply to an image.)

import { useTranslation } from "react-i18next"
import { IMAGEBLOCK_PICK_EVENT } from "../../extensions/ImageBlock"
import { StyleControls, TextInput } from "./StyleControls"
import type { ImageBlockPickRequest } from "../../extensions/ImageBlock"
import type { ParamFormProps, StyleField } from "./StyleControls"

const IMAGE_FIELDS: ReadonlyArray<StyleField> = ["textAlign", "borderRadius", "border"]

export function ImageParams({
  attrs,
  style,
  onStyleChange,
  onAttrsChange,
  disabled,
  tab,
  editor = null,
  pos = 0,
}: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  // Same event the image node view fires; VisualEmailEditor hosts the picker.
  const replace = () => {
    if (!editor) return
    const detail: ImageBlockPickRequest = {
      pos,
      mediaRef: (attrs.mediaRef as string | undefined) ?? "",
      alt: (attrs.alt as string | undefined) ?? "",
    }
    editor.view.dom.dispatchEvent(new CustomEvent<ImageBlockPickRequest>(IMAGEBLOCK_PICK_EVENT, { detail }))
  }
  return (
    <div className="ve-params-form" data-testid="ve-params-image">
      {tab !== "design" && (
        <>
          <button
            type="button"
            className="ve-params-action"
            data-testid="ve-param-replace-image"
            onClick={replace}
            disabled={disabled || editor === null}
          >
            {t("params.fields.replaceImage")}
          </button>
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
        </>
      )}
      {tab !== "content" && (
        <StyleControls value={style} fields={IMAGE_FIELDS} onChange={onStyleChange} disabled={disabled} />
      )}
    </div>
  )
}

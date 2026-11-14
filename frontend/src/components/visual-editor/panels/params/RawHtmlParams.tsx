// Parameters for the raw HTML block. It has no style; its Content tab just
// reopens the existing CodeMirror modal (hosted by VisualEmailEditor) for the
// selected block, using the same event the block's node view fires.

import { useTranslation } from "react-i18next"
import { RAWHTML_EDIT_EVENT } from "../../extensions/RawHTML"
import type { RawHTMLEditRequest } from "../../extensions/RawHTML"
import type { ParamFormProps } from "./StyleControls"

export function RawHtmlParams({ attrs, disabled, editor = null, pos = 0 }: ParamFormProps) {
  const { t } = useTranslation("visualEditor")
  const edit = () => {
    if (!editor) return
    const detail: RawHTMLEditRequest = { pos, html: (attrs.html as string | undefined) ?? "" }
    editor.view.dom.dispatchEvent(new CustomEvent<RawHTMLEditRequest>(RAWHTML_EDIT_EVENT, { detail }))
  }
  return (
    <div className="ve-params-form" data-testid="ve-params-rawhtml">
      <button
        type="button"
        className="ve-params-action"
        data-testid="ve-param-edit-html"
        onClick={edit}
        disabled={disabled || editor === null}
      >
        {t("params.fields.editHtml")}
      </button>
    </div>
  )
}

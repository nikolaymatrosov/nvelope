// BlockParamsPanel — the right pane of the three-pane editor (feature 017,
// T026). Shows the editable parameters for the currently-selected block, or a
// neutral empty state when nothing is selected (FR-012). Each change applies to
// the selected block only, live, via the shared selection model (FR-014); a
// whole-block "reset to default" clears the style back to theme inheritance
// (FR-019).

import { useTranslation } from "react-i18next"
import { isEmptyBlockStyle } from "../extensions/styleAttr"
import { ButtonParams } from "./params/ButtonParams"
import { ColumnsParams } from "./params/ColumnsParams"
import { DividerParams } from "./params/DividerParams"
import { ImageParams } from "./params/ImageParams"
import { TextParams } from "./params/TextParams"
import type { ParamFormProps } from "./params/StyleControls"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { BlockStyle } from "@/lib/api-types"

type Props = {
  selection: BlockSelection
  disabled?: boolean
}

const TEXT_TYPES = new Set(["paragraph", "heading", "blockquote", "bulletList", "orderedList"])
const STYLEABLE_TYPES = new Set([
  ...TEXT_TYPES,
  "button",
  "image",
  "divider",
  "columns",
  "column",
])

export function BlockParamsPanel({ selection, disabled = false }: Props) {
  const { t } = useTranslation("visualEditor")
  const node = selection.selectedNode

  return (
    <div className="ve-panel ve-panel--params" data-testid="ve-params-panel">
      <div className="ve-panel__header">
        <span className="ve-panel__title">{t("panel.paramsTitle")}</span>
        {node != null && STYLEABLE_TYPES.has(node.type.name) && (
          <button
            type="button"
            className="ve-panel__reset-all"
            data-testid="ve-params-reset-all"
            onClick={() => selection.updateSelectedAttrs({ style: null })}
            disabled={disabled}
          >
            {t("params.resetAll")}
          </button>
        )}
      </div>
      <PanelBody selection={selection} disabled={disabled} t={t} />
    </div>
  )
}

function PanelBody({
  selection,
  disabled,
  t,
}: {
  selection: BlockSelection
  disabled: boolean
  t: (k: string) => string
}) {
  const node = selection.selectedNode
  if (node == null) {
    return (
      <p className="ve-panel__empty" data-testid="ve-params-empty">
        {t("params.empty")}
      </p>
    )
  }

  const blockType = node.type.name
  if (!STYLEABLE_TYPES.has(blockType)) {
    return (
      <p className="ve-panel__empty" data-testid="ve-params-none">
        {blockType === "rawHtml" ? t("rawHtmlModal.title") : t("params.empty")}
      </p>
    )
  }

  const style = (node.attrs.style as BlockStyle | null) ?? {}

  const onStyleChange = (patch: Partial<BlockStyle>) => {
    const next = cleanStyle({ ...style, ...patch })
    selection.updateSelectedAttrs({ style: next })
  }
  const onAttrsChange = (patch: Record<string, unknown>) => {
    selection.updateSelectedAttrs(patch)
  }

  const props: ParamFormProps = {
    blockType,
    attrs: node.attrs,
    style,
    onStyleChange,
    onAttrsChange,
    disabled,
  }

  return (
    <div className="ve-panel__body" data-testid="ve-params-body" data-block-type={blockType}>
      <Form {...props} />
    </div>
  )
}

function Form(props: ParamFormProps) {
  if (props.blockType === "button") return <ButtonParams {...props} />
  if (props.blockType === "image") return <ImageParams {...props} />
  if (props.blockType === "divider") return <DividerParams {...props} />
  if (props.blockType === "columns" || props.blockType === "column") return <ColumnsParams {...props} />
  if (TEXT_TYPES.has(props.blockType)) return <TextParams {...props} />
  return null
}

// cleanStyle drops undefined/empty fields; an entirely-empty style collapses to
// null so the block reverts to theme inheritance and serializes without a
// `style` attr.
function cleanStyle(style: BlockStyle): BlockStyle | null {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(style as Record<string, unknown>)) {
    if (value != null && value !== "") out[key] = value
  }
  const result = out as BlockStyle
  return isEmptyBlockStyle(result) ? null : result
}

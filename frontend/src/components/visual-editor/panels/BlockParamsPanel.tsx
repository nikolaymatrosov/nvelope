// BlockParamsPanel — the settings half of the editor's right panel. Shows
// Content / Design tabs for the selected block (a tab with no fields is
// hidden). When nothing is selected, the root «Email» is current and the
// Design tab shows the caller's `root` content (the email theme controls).
// Every change applies live to the selected block through the shared
// selection model; "reset all" clears the block's style back to the theme.

import { useState } from "react"
import { useTranslation } from "react-i18next"
import { isEmptyBlockStyle } from "../extensions/styleAttr"
import { ButtonParams } from "./params/ButtonParams"
import { ColumnsParams } from "./params/ColumnsParams"
import { DividerParams } from "./params/DividerParams"
import { ImageParams } from "./params/ImageParams"
import { RawHtmlParams } from "./params/RawHtmlParams"
import { TextParams } from "./params/TextParams"
import type { ParamFormProps, ParamsTab } from "./params/StyleControls"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { ReactNode } from "react"
import type { BlockStyle } from "@/lib/api-types"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

type Props = {
  selection: BlockSelection
  editor?: Editor | null
  // Design-tab content for the root «Email» (nothing selected).
  root?: ReactNode
  disabled?: boolean
}

const TEXT_TYPES = new Set(["paragraph", "heading", "blockquote", "bulletList", "orderedList"])
const STYLEABLE_TYPES = new Set([...TEXT_TYPES, "button", "image", "divider", "columns", "column"])
const CONTENT_TYPES = new Set(["button", "image", "heading", "rawHtml"])

// tabsFor lists the settings tabs a selection has; null = root «Email».
export function tabsFor(blockType: string | null): Array<ParamsTab> {
  if (blockType === null) return ["design"]
  const tabs: Array<ParamsTab> = []
  if (CONTENT_TYPES.has(blockType)) tabs.push("content")
  if (STYLEABLE_TYPES.has(blockType)) tabs.push("design")
  return tabs
}

export function BlockParamsPanel({ selection, editor = null, root, disabled = false }: Props) {
  const { t } = useTranslation("visualEditor")
  const [chosen, setChosen] = useState<ParamsTab>("content")
  const node = selection.selectedNode
  const tabs = tabsFor(node ? node.type.name : null)
  // Keep the operator's last tab when the new selection has it.
  const active = tabs.includes(chosen) ? chosen : tabs[0]

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
      {tabs.length === 0 ? (
        <p className="ve-panel__empty" data-testid="ve-params-none">
          {t("params.empty")}
        </p>
      ) : (
        <Tabs value={active} onValueChange={(v) => setChosen(v as ParamsTab)} className="ve-panel__tabs">
          <TabsList>
            {tabs.map((tab) => (
              <TabsTrigger key={tab} value={tab} data-testid={`ve-params-tab-${tab}`}>
                {t(`params.tabs.${tab}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent
            value={active}
            className="ve-panel__body"
            data-testid="ve-params-body"
            data-block-type={node ? node.type.name : "root"}
          >
            {node == null ? (
              (root ?? (
                <p className="ve-panel__empty" data-testid="ve-params-empty">
                  {t("params.empty")}
                </p>
              ))
            ) : (
              <BlockForm
                // Remount per block so switches/drafts don't leak between blocks.
                key={selection.selectedPos ?? 0}
                node={node}
                pos={selection.selectedPos ?? 0}
                tab={active}
                selection={selection}
                editor={editor}
                disabled={disabled}
              />
            )}
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}

function BlockForm({
  node,
  pos,
  tab,
  selection,
  editor,
  disabled,
}: {
  node: PMNode
  pos: number
  tab: ParamsTab
  selection: BlockSelection
  editor: Editor | null
  disabled: boolean
}) {
  const style = (node.attrs.style as BlockStyle | null) ?? {}
  const props: ParamFormProps = {
    blockType: node.type.name,
    attrs: node.attrs,
    style,
    onStyleChange: (patch) => selection.updateSelectedAttrs({ style: cleanStyle({ ...style, ...patch }) }),
    onAttrsChange: (patch) => selection.updateSelectedAttrs(patch),
    disabled,
    tab,
    editor,
    pos,
  }
  const type = props.blockType
  if (type === "button") return <ButtonParams {...props} />
  if (type === "image") return <ImageParams {...props} />
  if (type === "divider") return <DividerParams {...props} />
  if (type === "columns" || type === "column") return <ColumnsParams {...props} />
  if (type === "rawHtml") return <RawHtmlParams {...props} />
  if (TEXT_TYPES.has(type)) return <TextParams {...props} />
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

// ThreePaneEditor — the three-pane shell hosting the VisualEmailEditor canvas
// in the center, the StructureOutline on the left, and the BlockParamsPanel on
// the right (FR-001). It owns the single useBlockSelection state shared by all
// three panes (FR-002).
//
// US3 adds the chrome: each side panel is independently collapsible/expandable
// (FR-020), the layout (sizes + collapsed state) is remembered per operator via
// localStorage (FR-021), and on a narrow viewport the side panels become
// overlays so the canvas stays usable (FR-022).

import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useDefaultLayout, usePanelRef } from "react-resizable-panels"
import { VisualEmailEditor } from "./VisualEmailEditor"
import { useBlockSelection } from "./hooks/useBlockSelection"
import { BlockParamsPanel } from "./panels/BlockParamsPanel"
import { StructureOutline } from "./panels/StructureOutline"
import type { Editor } from "@tiptap/core"
import type { TFunction } from "i18next"
import type { ComponentProps, ReactNode } from "react"
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet"
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable"

const NARROW_QUERY = "(max-width: 1024px)"

// useIsNarrow tracks whether the viewport is too narrow for three side-by-side
// panes. Defaults to false (wide) so server render / no-matchMedia environments
// get the full layout.
function useIsNarrow(): boolean {
  const [narrow, setNarrow] = useState(false)
  useEffect(() => {
    // useEffect only runs in the browser, so window.matchMedia is available.
    const mq = window.matchMedia(NARROW_QUERY)
    const update = () => setNarrow(mq.matches)
    update()
    mq.addEventListener("change", update)
    return () => mq.removeEventListener("change", update)
  }, [])
  return narrow
}

type Props = Omit<ComponentProps<typeof VisualEmailEditor>, "onEditorReady">

export function ThreePaneEditor(props: Props) {
  const { t } = useTranslation("visualEditor")
  const [editor, setEditor] = useState<Editor | null>(null)
  const selection = useBlockSelection(editor)
  const editable = props.editable ?? true
  const narrow = useIsNarrow()

  const left = editor ? <StructureOutline editor={editor} selection={selection} /> : null
  const right = <BlockParamsPanel selection={selection} disabled={!editable} />
  const canvas = <VisualEmailEditor {...props} onEditorReady={setEditor} />

  if (narrow) {
    return <NarrowLayout left={left} right={right} canvas={canvas} t={t} />
  }
  return <WideLayout left={left} right={right} canvas={canvas} t={t} />
}

type LayoutProps = {
  left: ReactNode
  right: ReactNode
  canvas: ReactNode
  t: TFunction<"visualEditor">
}

// WideLayout: three resizable, individually-collapsible panels with persisted
// layout. The toggle buttons drive each side panel's imperative collapse/expand.
function WideLayout({ left, right, canvas, t }: LayoutProps) {
  const leftRef = usePanelRef()
  const rightRef = usePanelRef()
  const [leftCollapsed, setLeftCollapsed] = useState(false)
  const [rightCollapsed, setRightCollapsed] = useState(false)

  const storage = typeof window !== "undefined" ? window.localStorage : undefined
  const { defaultLayout, onLayoutChanged } = useDefaultLayout({
    id: "ve-three-pane",
    storage,
    panelIds: ["ve-pane-left", "ve-pane-center", "ve-pane-right"],
  })

  const toggle = (ref: typeof leftRef) => {
    const panel = ref.current
    if (!panel) return
    if (panel.isCollapsed()) panel.expand()
    else panel.collapse()
  }

  return (
    <div className="ve-three-pane" data-testid="ve-three-pane">
      <div className="ve-three-pane__bar">
        <button
          type="button"
          className="ve-three-pane__bar-btn"
          data-testid="ve-toggle-left"
          aria-expanded={!leftCollapsed}
          onClick={() => toggle(leftRef)}
        >
          {leftCollapsed ? t("panel.expandLeft") : t("panel.collapseLeft")}
        </button>
        <button
          type="button"
          className="ve-three-pane__bar-btn"
          data-testid="ve-toggle-right"
          aria-expanded={!rightCollapsed}
          onClick={() => toggle(rightRef)}
        >
          {rightCollapsed ? t("panel.expandRight") : t("panel.collapseRight")}
        </button>
      </div>
      <ResizablePanelGroup
        orientation="horizontal"
        className="ve-three-pane__group"
        defaultLayout={defaultLayout}
        onLayoutChanged={onLayoutChanged}
      >
        <ResizablePanel
          id="ve-pane-left"
          panelRef={leftRef}
          collapsible
          collapsedSize={0}
          minSize={12}
          defaultSize={20}
          className="ve-pane ve-pane--left"
          onResize={(size) => setLeftCollapsed(size.asPercentage === 0)}
        >
          {left}
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel
          id="ve-pane-center"
          minSize={30}
          defaultSize={56}
          className="ve-pane ve-pane--center"
        >
          {canvas}
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel
          id="ve-pane-right"
          panelRef={rightRef}
          collapsible
          collapsedSize={0}
          minSize={16}
          defaultSize={24}
          className="ve-pane ve-pane--right"
          onResize={(size) => setRightCollapsed(size.asPercentage === 0)}
        >
          {right}
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  )
}

// NarrowLayout: the canvas fills the width; the side panels open as overlays
// (sheets) from a top bar so the canvas never shrinks below usability (FR-022).
function NarrowLayout({ left, right, canvas, t }: LayoutProps) {
  return (
    <div className="ve-three-pane ve-three-pane--narrow" data-testid="ve-three-pane">
      <div className="ve-three-pane__bar">
        <Sheet>
          <SheetTrigger className="ve-three-pane__bar-btn" data-testid="ve-open-left">
            {t("panel.expandLeft")}
          </SheetTrigger>
          <SheetContent side="left" className="ve-overlay-panel">
            <SheetTitle>{t("panel.structureTitle")}</SheetTitle>
            {left}
          </SheetContent>
        </Sheet>
        <Sheet>
          <SheetTrigger className="ve-three-pane__bar-btn" data-testid="ve-open-right">
            {t("panel.expandRight")}
          </SheetTrigger>
          <SheetContent side="right" className="ve-overlay-panel">
            <SheetTitle>{t("panel.paramsTitle")}</SheetTitle>
            {right}
          </SheetContent>
        </Sheet>
      </div>
      <div className="ve-pane ve-pane--center" data-testid="ve-pane-center">
        {canvas}
      </div>
    </div>
  )
}

// EditorScreen — the full-screen, site-builder style email editor (phase 1 of
// docs/superpowers/specs/2026-11-14-editor-shell-design.md). The
// /templates/$id/edit and /campaigns/$id/edit routes render it with the loaded
// row plus entity-specific convert / save / refetch functions.
//
// EditorScreen resolves the starting document: a legacy raw-HTML row is
// converted on load and opened as an unsaved document (nothing persists until
// Save). EditorSession then owns the session: save state (useVisualSave), the
// TipTap canvas, the shared block selection, and the layout. The layout is the
// top bar, the dotted canvas with the white page, the right panel (Layers over
// Content/Design), the block library sheet, and the preview dialog.

import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { useBlocker } from "@tanstack/react-router"
import { useEditorState } from "@tiptap/react"
import { LayoutTemplate } from "lucide-react"
import { useDefaultLayout } from "react-resizable-panels"
import { toast } from "sonner"
import { VisualEmailEditor } from "./VisualEmailEditor"
import { insertBlock } from "./blockCatalog"
import { useBlockSelection } from "./hooks/useBlockSelection"
import { useVisualSave } from "./hooks/useVisualSave"
import { BlockLibrary } from "./panels/BlockLibrary"
import { BlockParamsPanel } from "./panels/BlockParamsPanel"
import { StructureOutline } from "./panels/StructureOutline"
import { useEditorTheme } from "./plugins/theming"
import { BlockToolbar } from "./ui/BlockToolbar"
import { EditorTopBar } from "./ui/EditorTopBar"
import { PreviewIframe } from "./ui/PreviewIframe"
import { ThemeControls } from "./ui/ThemeControls"
import type { CatalogItem } from "./blockCatalog"
import type { BlockSelection } from "./hooks/useBlockSelection"
import type { VisualSaveFn } from "./hooks/useVisualSave"
import type { Device } from "./ui/EditorTopBar"
import type { Editor } from "@tiptap/core"
import type { ReactNode } from "react"
import type { ConvertToVisualResponse, Theme, VisualDoc } from "@/lib/api-types"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"

export type EditorRow = {
  id: string
  name: string
  body_doc?: VisualDoc | null
  body_html: string
  theme?: Theme | null
  updated_at: string
}

export type EditorScreenProps = {
  slug: string
  // Breadcrumb root label ("Templates" / "Campaigns").
  section: string
  row: EditorRow
  editable: boolean
  onBack: () => void
  convert: () => Promise<{ data: ConvertToVisualResponse }>
  save: VisualSaveFn
  refetch: () => Promise<EditorRow>
  onSaved: () => Promise<void>
}

const EMPTY_DOC: VisualDoc = { version: 1, type: "doc", content: [{ type: "paragraph", content: [] }] }
const MOBILE_WIDTH = 375
const NARROW_QUERY = "(width < 1024px)"

// useIsNarrow: too narrow for canvas + side panel side by side. Defaults to
// wide; stays wide where matchMedia is missing.
function useIsNarrow(): boolean {
  const [narrow, setNarrow] = useState(false)
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return
    const mq = window.matchMedia(NARROW_QUERY)
    const update = () => setNarrow(mq.matches)
    update()
    mq.addEventListener("change", update)
    return () => mq.removeEventListener("change", update)
  }, [])
  return narrow
}

function layoutStorage(): Storage | undefined {
  return typeof window !== "undefined" ? window.localStorage : undefined
}

export function EditorScreen(props: EditorScreenProps) {
  const { t } = useTranslation("visualEditor")
  const { slug, row, convert, onBack } = props
  const needsConvert = row.body_doc == null && row.body_html.trim() !== ""

  const conversion = useQuery({
    queryKey: ["t", slug, "visual-convert", row.id],
    queryFn: async () => {
      const res = await convert()
      const warnings = res.data.warnings.length
      if (warnings > 0) {
        toast.warning(t("save.convertedWarnings", { count: warnings }), { duration: 12_000 })
      } else {
        toast.success(t("save.convertedSuccess"))
      }
      return res.data.bodyDoc
    },
    enabled: needsConvert,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  })

  if (needsConvert && conversion.isError) {
    return (
      <div className="ve-screen__notice" data-testid="ve-convert-failed">
        <p>{t("screen.convertFailed")}</p>
        <Button type="button" variant="outline" onClick={onBack}>
          {t("screen.back")}
        </Button>
      </div>
    )
  }
  if (needsConvert && conversion.isPending) {
    return (
      <p className="ve-screen__notice" data-testid="ve-converting">
        {t("screen.converting")}
      </p>
    )
  }

  return (
    <EditorSession
      {...props}
      initialDoc={conversion.data ?? row.body_doc ?? EMPTY_DOC}
      initiallyDirty={needsConvert}
    />
  )
}

function EditorSession({
  slug,
  section,
  row,
  editable,
  onBack,
  save,
  refetch,
  onSaved,
  initialDoc,
  initiallyDirty,
}: EditorScreenProps & { initialDoc: VisualDoc; initiallyDirty: boolean }) {
  const { t } = useTranslation("visualEditor")
  const session = useVisualSave({
    initial: { bodyDoc: initialDoc, theme: row.theme ?? null, updatedAt: row.updated_at },
    initiallyDirty,
    editable,
    save,
    refetch: async () => {
      const fresh = await refetch()
      return { bodyDoc: fresh.body_doc ?? EMPTY_DOC, theme: fresh.theme ?? null, updatedAt: fresh.updated_at }
    },
    onSaved,
  })
  const [editor, setEditor] = useState<Editor | null>(null)
  const selection = useBlockSelection(editor)
  const [device, setDevice] = useState<Device>("desktop")
  const [libraryOpen, setLibraryOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  const narrow = useIsNarrow()
  const pageRef = useRef<HTMLDivElement | null>(null)
  const { theme: resolvedTheme } = useEditorTheme(slug, session.theme)
  const { defaultLayout, onLayoutChanged } = useDefaultLayout({
    id: "ve-screen",
    storage: layoutStorage(),
    panelIds: ["ve-main", "ve-side-panel"],
  })
  const history = useEditorState({
    editor,
    selector: (ctx) => ({
      canUndo: ctx.editor?.can().undo() ?? false,
      canRedo: ctx.editor?.can().redo() ?? false,
    }),
  })

  // Unsaved changes: confirm in-app navigation, warn on tab close.
  useBlocker({
    shouldBlockFn: () => session.dirty && !window.confirm(t("screen.unsavedConfirm")),
    enableBeforeUnload: () => session.dirty,
  })

  // Cmd/Ctrl+S saves; Esc returns to the root «Email». ProseMirror always
  // preventDefault()s Esc inside the canvas, so an Esc from the canvas always
  // clears (the slash menu stops propagation when it consumes Esc). Elsewhere a
  // preventDefault()ed Esc was a Radix overlay closing, so the selection stays.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault()
        if (editable && session.dirty && !session.saving) session.save()
        return
      }
      if (e.key !== "Escape") return
      const fromCanvas = e.target instanceof Node && editor?.view.dom.contains(e.target) === true
      if (fromCanvas || !e.defaultPrevented) selection.clear()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [editable, editor, session, selection])

  const onInsert = (item: CatalogItem) => {
    if (!editor) return
    const pos = insertBlock(editor, selection.selectedPos, item.content())
    selection.selectBlock(pos)
    setLibraryOpen(false)
  }

  const pageWidth = device === "mobile" ? MOBILE_WIDTH : resolvedTheme.containerWidth
  const isEmpty = editor ? editor.isEmpty : false

  const canvas = (
    <div className="ve-canvas" data-testid="ve-canvas">
      <div
        ref={pageRef}
        className={`ve-page${selection.selectedPos !== null ? " ve-has-selection" : ""}`}
        style={{ width: pageWidth }}
        data-testid="ve-page"
      >
        <VisualEmailEditor
          slug={slug}
          value={session.bodyDoc}
          onChange={session.setBodyDoc}
          theme={session.theme}
          editable={editable}
          onEditorReady={setEditor}
        />
        {isEmpty && editable && (
          <div className="ve-empty" data-testid="ve-empty">
            <LayoutTemplate className="size-8" aria-hidden />
            <p>{t("screen.emptyTitle")}</p>
            <Button type="button" variant="outline" onClick={() => setLibraryOpen(true)} data-testid="ve-empty-add">
              {t("screen.emptyAction")}
            </Button>
          </div>
        )}
        {editor && editable && <BlockToolbar editor={editor} selection={selection} pageRef={pageRef} />}
      </div>
    </div>
  )

  const side = (
    <SidePanel
      editor={editor}
      selection={selection}
      editable={editable}
      root={
        <ThemeControls
          value={session.theme}
          resolved={resolvedTheme}
          onChange={session.setTheme}
          disabled={!editable}
        />
      }
    />
  )

  return (
    <div className="ve-screen" data-testid="ve-screen">
      <EditorTopBar
        section={section}
        name={row.name}
        onBack={onBack}
        editable={editable}
        device={device}
        onDeviceChange={setDevice}
        canUndo={history?.canUndo ?? false}
        canRedo={history?.canRedo ?? false}
        onUndo={() => editor?.chain().focus().undo().run()}
        onRedo={() => editor?.chain().focus().redo().run()}
        onAdd={() => setLibraryOpen(true)}
        onPreview={() => setPreviewOpen(true)}
        dirty={session.dirty}
        saving={session.saving}
        onSave={session.save}
        onOpenPanel={narrow ? () => setPanelOpen(true) : undefined}
      />
      {narrow ? (
        <>
          <div className="ve-screen__body">{canvas}</div>
          <Sheet open={panelOpen} onOpenChange={setPanelOpen}>
            <SheetContent side="right" className="ve-overlay-panel" aria-describedby={undefined}>
              <SheetTitle className="sr-only">{t("screen.openPanel")}</SheetTitle>
              {side}
            </SheetContent>
          </Sheet>
        </>
      ) : (
        <ResizablePanelGroup
          orientation="horizontal"
          className="ve-screen__body"
          defaultLayout={defaultLayout}
          onLayoutChanged={onLayoutChanged}
        >
          {/* react-resizable-panels v4: numbers are pixels, so sizes are "%" strings. */}
          <ResizablePanel id="ve-main" defaultSize="74%" minSize="40%">
            {canvas}
          </ResizablePanel>
          <ResizableHandle withHandle />
          <ResizablePanel id="ve-side-panel" defaultSize="26%" minSize="18%" maxSize="45%" className="ve-screen__side">
            {side}
          </ResizablePanel>
        </ResizablePanelGroup>
      )}
      <BlockLibrary open={libraryOpen} onOpenChange={setLibraryOpen} onInsert={onInsert} />
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent aria-describedby={undefined} className="flex h-[90vh] flex-col sm:max-w-5xl" data-testid="ve-preview-dialog">
          <DialogTitle>{t("screen.previewTitle")}</DialogTitle>
          {previewOpen && <PreviewIframe slug={slug} doc={session.bodyDoc} theme={session.theme} sample={null} />}
        </DialogContent>
      </Dialog>
    </div>
  )
}

// SidePanel: Layers over Content/Design, split vertically (height persisted).
function SidePanel({
  editor,
  selection,
  editable,
  root,
}: {
  editor: Editor | null
  selection: BlockSelection
  editable: boolean
  root: ReactNode
}) {
  const { defaultLayout, onLayoutChanged } = useDefaultLayout({
    id: "ve-side-split",
    storage: layoutStorage(),
    panelIds: ["ve-layers", "ve-settings"],
  })
  return (
    <ResizablePanelGroup
      orientation="vertical"
      className="ve-side"
      defaultLayout={defaultLayout}
      onLayoutChanged={onLayoutChanged}
    >
      <ResizablePanel id="ve-layers" defaultSize="40%" minSize="15%" className="ve-side__pane">
        {editor ? <StructureOutline editor={editor} selection={selection} editable={editable} /> : null}
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel id="ve-settings" defaultSize="60%" minSize="25%" className="ve-side__pane">
        <BlockParamsPanel selection={selection} editor={editor} root={root} disabled={!editable} />
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}

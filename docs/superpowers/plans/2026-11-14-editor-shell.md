# Visual Editor Shell (site-builder style) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the inline three-pane email editor with a full-screen, site-builder style editor (top bar, dotted canvas, Layers + Content/Design panel on the right, slide-out block library) for templates and campaigns, without touching the data model, BFF, or Go.

**Architecture:** New non-nested routes `/t/$slug/{templates,campaigns}/$id/edit` render a shared `EditorScreen` full-screen (`AppShell` drops its chrome via `staticData.fullscreen`). `EditorScreen` converts legacy raw-HTML rows on load, then owns the session: `useVisualSave` (doc/theme/dirty/save + 409 flow), the TipTap canvas (`VisualEmailEditor`, unchanged except dead props removed), `useBlockSelection`, and the layout pieces (`EditorTopBar`, `BlockLibrary`, `StructureOutline`, `BlockParamsPanel`, `BlockToolbar`). Detail pages keep name/subject and show a `body_html` preview plus an «Open editor» link.

**Tech Stack:** React 19, TypeScript 5.9, TanStack Router/Query, TipTap 3, shadcn/ui (Radix), `react-resizable-panels` v4, lucide-react, react-i18next, Vitest + Testing Library (jsdom), Storybook 10.

**Spec:** [docs/superpowers/specs/2026-11-14-editor-shell-design.md](../specs/2026-11-14-editor-shell-design.md)

## Global Constraints

- No changes to the `VisualDoc` wire format, the BFF (`frontend/src/server/`), or Go (`internal/`). `BlockStyle` field sets and bounds stay exactly as in feature 017.
- No new npm dependency. Use the existing shadcn primitives (`tabs`, `sheet`, `dialog`, `dropdown-menu`, `resizable`, `badge`, `button`) and lucide-react icons.
- Every user-visible string goes through i18n (`visualEditor` namespace), en + ru, with **identical key sets** (enforced by `src/i18n/catalog-parity.test.ts`; plural keys need `_one/_few/_many/_other` in both locales).
- After editing `src/locales/en/*.json`, regenerate the typed resources: `pnpm i18n:types` (rewrites `src/i18n/resources.d.ts`; commit it).
- Lint is strict (`@tanstack/eslint-config`, `--max-warnings 0`): `Array<T>` not `T[]`, no unnecessary conditions, sorted imports. Run `pnpm exec eslint --fix <files>` before committing.
- Commit messages: conventional style (`feat(visual-editor): …`, `test(…)`, `refactor(…)`). **Never add a `Co-Authored-By` trailer.**
- Unchanged in the editor: in-canvas `DragHandle`, text bubble menu, merge-tag picker, media picker, raw-HTML modal.

## Environment

- `node`/`pnpm` are nvm lazy-loaded in non-interactive shells. Prefix every pnpm command with
  `export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" &&`.
- Run tests from the repo root with the wrapper (no permission prompt, prints `EXIT:<code>`):
  `.claude/scripts/test-frontend.sh <path relative to frontend/>`. It runs both the jsdom `unit` project and the Storybook browser project. If Chromium is missing: `cd frontend && pnpm exec playwright install chromium`.
- Typecheck / lint: `cd frontend && pnpm typecheck && pnpm lint` (then `cd ..`). `cd` persists between Bash calls.
- Paths containing `$` must be single-quoted in shell commands, e.g. `'frontend/src/routes/t/$slug/templates/$id.tsx'`.
- Work on branch `feat/editor-shell` created from `main`. First commit: the spec and this plan.

## File Map

| File | Status | Responsibility |
| --- | --- | --- |
| `frontend/src/locales/{en,ru}/visualEditor.json`, `frontend/src/i18n/resources.d.ts` | modify | New strings; generated types |
| `frontend/src/components/visual-editor/hooks/useBlockSelection.ts` | modify | Selection starts empty; `clear()` sticks |
| `frontend/src/components/visual-editor/blockCatalog.ts` | create | Shared insertables + `insertBlock()` |
| `frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx` | modify | Items from the catalog, i18n labels (`slashItems()`) |
| `frontend/src/components/visual-editor/extensions/Columns.tsx` | modify | `setColumnsCount()` |
| `frontend/src/components/visual-editor/panels/params/StyleControls.tsx` | modify | Grouped sections, switches, padding row, `ParamsTab`, extended `ParamFormProps` |
| `frontend/src/components/visual-editor/panels/params/{Button,Text,Image,Columns}Params.tsx` | modify | Render only the active tab's fields |
| `frontend/src/components/visual-editor/panels/params/RawHtmlParams.tsx` | create | «Edit HTML» action |
| `frontend/src/components/visual-editor/panels/BlockParamsPanel.tsx` | modify | Content/Design tabs, root slot, `tabsFor()` |
| `frontend/src/components/visual-editor/panels/StructureOutline.tsx` | modify | Root «Email» row, ⋮ menu, `editable`, exported helpers |
| `frontend/src/components/visual-editor/panels/BlockLibrary.tsx` | create | Slide-out library |
| `frontend/src/components/visual-editor/ui/BlockToolbar.tsx` | create | Floating label under the selected block |
| `frontend/src/components/visual-editor/hooks/useVisualSave.ts` | create | Session save state + 409 flow |
| `frontend/src/components/visual-editor/ui/EditorTopBar.tsx` | create | Top bar |
| `frontend/src/components/visual-editor/EditorScreen.tsx` | create | Conversion + session + layout |
| `frontend/src/components/visual-editor/VisualBodyPreview.tsx` | create | Detail-page preview of saved `body_html` |
| `frontend/src/components/visual-editor/visual-editor.css` | modify | Screen, canvas, toolbar, library, controls |
| `frontend/src/components/shell/app-shell.tsx` | modify | Fullscreen routes skip chrome |
| `frontend/src/routes/t/$slug/templates/$id_.edit.tsx`, `campaigns/$id_.edit.tsx` | create | Edit routes |
| `frontend/src/routes/t/$slug/templates/$id.tsx`, `campaigns/$id.tsx` | modify | Inline editor → preview + link |
| `frontend/src/routeTree.gen.ts` | regenerate | Route tree |
| `frontend/src/components/visual-editor/ThreePaneEditor{,.test,.stories}.tsx` | delete | Replaced by `EditorScreen` |
| `frontend/src/components/visual-editor/VisualEmailEditor.tsx` | modify | Remove unused toolbar/theme props |

---

### Task 1: i18n strings for the editor shell

**Files:**
- Modify: `frontend/src/locales/en/visualEditor.json`
- Modify: `frontend/src/locales/ru/visualEditor.json`
- Regenerate: `frontend/src/i18n/resources.d.ts`
- Test: `frontend/src/i18n/catalog-parity.test.ts` (existing)

**Interfaces:**
- Produces: the `visualEditor` keys every later task uses — `screen.*`, `save.*`, `library.*`, `catalog.*`, `body.*`, `params.tabs.*`, `params.toggle.*`, `params.fields.{columnCount,replaceImage,editHtml}`, `structure.actions.menu`.

- [ ] **Step 1: Add the English strings**

In `frontend/src/locales/en/visualEditor.json`:

1. Change existing values: `panel.structureTitle` → `"Layers"`; `params.sections.spacing` → `"Spacing & alignment"`; `params.fields.paddingTop` → `"Top"`, `paddingRight` → `"Right"`, `paddingBottom` → `"Bottom"`, `paddingLeft` → `"Left"`.
2. Add inside `structure.actions`: `"menu": "Block actions"`.
3. Add inside `params`: 
```json
"tabs": { "content": "Content", "design": "Design" },
"toggle": { "border": "Border", "radius": "Rounded corners" },
```
4. Add inside `params.fields`: `"columnCount": "Number of columns"`, `"replaceImage": "Replace image"`, `"editHtml": "Edit HTML"`.
5. Add these top-level objects:
```json
"screen": {
  "breadcrumb": { "templates": "Templates", "campaigns": "Campaigns" },
  "back": "Back",
  "add": "Add block",
  "undo": "Undo",
  "redo": "Redo",
  "device": { "label": "Viewport", "desktop": "Desktop", "mobile": "Mobile" },
  "preview": "Preview",
  "previewTitle": "Email preview",
  "save": "Save",
  "saving": "Saving…",
  "saved": "Saved",
  "readOnly": "View only",
  "openPanel": "Properties",
  "emptyTitle": "Add your first block",
  "emptyAction": "Add block",
  "unsavedConfirm": "You have unsaved changes. Leave anyway?",
  "converting": "Converting to the visual editor…",
  "convertFailed": "Could not convert this email to the visual editor.",
  "unavailable": "The visual editor is not available for transactional templates.",
  "rootLabel": "Email"
},
"save": {
  "success": "Saved.",
  "warnings_one": "Saved with {{count}} content warning.",
  "warnings_few": "Saved with {{count}} content warnings.",
  "warnings_many": "Saved with {{count}} content warnings.",
  "warnings_other": "Saved with {{count}} content warnings.",
  "staleTitle": "Changed in another tab/session",
  "reload": "Reload",
  "forceOverwrite": "Force overwrite",
  "convertedSuccess": "Converted to the visual editor. Review and save to keep it.",
  "convertedWarnings_one": "Converted with {{count}} block preserved as raw HTML. Review and save to keep the visual document.",
  "convertedWarnings_few": "Converted with {{count}} blocks preserved as raw HTML. Review and save to keep the visual document.",
  "convertedWarnings_many": "Converted with {{count}} blocks preserved as raw HTML. Review and save to keep the visual document.",
  "convertedWarnings_other": "Converted with {{count}} blocks preserved as raw HTML. Review and save to keep the visual document."
},
"library": {
  "title": "Block library",
  "group": { "basic": "Basic" }
},
"catalog": {
  "type": {
    "text": "Text",
    "heading": "Heading",
    "image": "Image",
    "button": "Button",
    "columns": "Columns",
    "divider": "Divider",
    "html": "HTML"
  },
  "item": {
    "paragraph": "Paragraph",
    "bullet-list": "Bulleted list",
    "ordered-list": "Numbered list",
    "quote": "Quote",
    "code": "Code block",
    "heading-1": "Heading 1",
    "heading-2": "Heading 2",
    "heading-3": "Heading 3",
    "image": "Image",
    "button": "Button",
    "columns-2": "Two columns",
    "columns-3": "Three columns",
    "columns-4": "Four columns",
    "divider": "Divider",
    "raw-html": "HTML block",
    "merge-tag": "Merge tag"
  },
  "mergeTagDescription": "Insert a subscriber or campaign placeholder"
},
"body": {
  "previewTitle": "Saved email",
  "previewEmpty": "The email body is empty.",
  "openEditor": "Open editor",
  "editHtmlOnly": "Edit as HTML only"
}
```

- [ ] **Step 2: Add the Russian strings (same keys)**

In `frontend/src/locales/ru/visualEditor.json`:

1. Change: `panel.structureTitle` → `"Слои"`; `params.sections.spacing` → `"Отступы и выравнивание"`; `params.fields.paddingTop` → `"Сверху"`, `paddingRight` → `"Справа"`, `paddingBottom` → `"Снизу"`, `paddingLeft` → `"Слева"`.
2. `structure.actions.menu`: `"Действия с блоком"`.
3. `params.tabs`: `{ "content": "Контент", "design": "Дизайн" }`; `params.toggle`: `{ "border": "Обводка", "radius": "Скругление" }`.
4. `params.fields`: `"columnCount": "Количество колонок"`, `"replaceImage": "Заменить изображение"`, `"editHtml": "Редактировать HTML"`.
5. Top-level:
```json
"screen": {
  "breadcrumb": { "templates": "Шаблоны", "campaigns": "Кампании" },
  "back": "Назад",
  "add": "Добавить блок",
  "undo": "Отменить",
  "redo": "Повторить",
  "device": { "label": "Экран", "desktop": "Компьютер", "mobile": "Телефон" },
  "preview": "Предпросмотр",
  "previewTitle": "Предпросмотр письма",
  "save": "Сохранить",
  "saving": "Сохранение…",
  "saved": "Сохранено",
  "readOnly": "Только просмотр",
  "openPanel": "Свойства",
  "emptyTitle": "Добавьте первый блок",
  "emptyAction": "Добавить блок",
  "unsavedConfirm": "Есть несохранённые изменения. Всё равно уйти?",
  "converting": "Преобразование в визуальный редактор…",
  "convertFailed": "Не удалось преобразовать письмо для визуального редактора.",
  "unavailable": "Визуальный редактор недоступен для транзакционных шаблонов.",
  "rootLabel": "Письмо"
},
"save": {
  "success": "Сохранено.",
  "warnings_one": "Сохранено с {{count}} предупреждением о содержимом.",
  "warnings_few": "Сохранено с {{count}} предупреждениями о содержимом.",
  "warnings_many": "Сохранено с {{count}} предупреждениями о содержимом.",
  "warnings_other": "Сохранено с {{count}} предупреждениями о содержимом.",
  "staleTitle": "Изменено в другой вкладке или сеансе",
  "reload": "Перезагрузить",
  "forceOverwrite": "Перезаписать принудительно",
  "convertedSuccess": "Преобразовано в визуальный редактор. Проверьте и сохраните, чтобы оставить изменения.",
  "convertedWarnings_one": "Преобразовано, {{count}} блок сохранён как необработанный HTML. Проверьте и сохраните, чтобы оставить визуальный документ.",
  "convertedWarnings_few": "Преобразовано, {{count}} блока сохранены как необработанный HTML. Проверьте и сохраните, чтобы оставить визуальный документ.",
  "convertedWarnings_many": "Преобразовано, {{count}} блоков сохранены как необработанный HTML. Проверьте и сохраните, чтобы оставить визуальный документ.",
  "convertedWarnings_other": "Преобразовано, {{count}} блоков сохранены как необработанный HTML. Проверьте и сохраните, чтобы оставить визуальный документ."
},
"library": {
  "title": "Библиотека блоков",
  "group": { "basic": "Основные" }
},
"catalog": {
  "type": {
    "text": "Текст",
    "heading": "Заголовок",
    "image": "Изображение",
    "button": "Кнопка",
    "columns": "Колонки",
    "divider": "Разделитель",
    "html": "HTML"
  },
  "item": {
    "paragraph": "Абзац",
    "bullet-list": "Маркированный список",
    "ordered-list": "Нумерованный список",
    "quote": "Цитата",
    "code": "Код",
    "heading-1": "Заголовок 1",
    "heading-2": "Заголовок 2",
    "heading-3": "Заголовок 3",
    "image": "Изображение",
    "button": "Кнопка",
    "columns-2": "Две колонки",
    "columns-3": "Три колонки",
    "columns-4": "Четыре колонки",
    "divider": "Разделитель",
    "raw-html": "HTML-блок",
    "merge-tag": "Переменная"
  },
  "mergeTagDescription": "Вставить переменную подписчика или кампании"
},
"body": {
  "previewTitle": "Сохранённое письмо",
  "previewEmpty": "Тело письма пусто.",
  "openEditor": "Открыть редактор",
  "editHtmlOnly": "Редактировать только как HTML"
}
```

- [ ] **Step 3: Regenerate typed resources**

Run: `cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm i18n:types && cd ..`
Expected: `src/i18n/resources.d.ts` now contains `"screen"`, `"catalog"`, `"library"`.

- [ ] **Step 4: Run the parity test**

Run: `.claude/scripts/test-frontend.sh src/i18n`
Expected: `EXIT:0`. If parity fails, the message names the key missing on one side. Fix it there.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/locales/en/visualEditor.json frontend/src/locales/ru/visualEditor.json frontend/src/i18n/resources.d.ts
git commit -m "feat(visual-editor): add i18n strings for the full-screen editor shell"
```

---

### Task 2: Selection starts empty and `clear()` sticks

The new UI has a root «Email» node that is "current" when no block is selected. ProseMirror always holds some selection, so `useBlockSelection` gets an *active* flag: false on load and after `clear()`, true after real user intent (mousedown/keydown on the canvas, editor focus, or `selectBlock`). Programmatic transactions alone (e.g. a reload's `setContent`) never re-select.

**Files:**
- Modify: `frontend/src/components/visual-editor/hooks/useBlockSelection.ts`
- Test: `frontend/src/components/visual-editor/hooks/useBlockSelection.test.tsx`

**Interfaces:**
- Produces: `BlockSelection` (type unchanged). `selectedPos === null` now means "root / nothing selected" and holds until the user selects. `clear()` is sticky.

- [ ] **Step 1: Write the failing tests**

In `useBlockSelection.test.tsx`:

1. Change the import to `import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react"`.
2. Add `clear: () => void` to `HarnessHandle`, pass `clear: selection.clear` in `Harness`'s `onState(...)`, and `clear: () => {}` in `renderHarness`'s initial `latest`.
3. In the existing test `"derives the block containing the cursor"`, insert `fireEvent.mouseDown(editor.view.dom)` on the line before `editor.commands.setTextSelection(2)`.
4. Append these tests inside `describe("useBlockSelection", ...)`:

```tsx
  it("reports nothing selected until the operator selects", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    expect(get().selectedPos).toBeNull()
    expect(get().selectedType).toBe("none")
  })

  it("activates on a mousedown in the canvas", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    fireEvent.mouseDown(get().editor!.view.dom)
    await waitFor(() => expect(get().selectedType).toBe("paragraph"))
  })

  it("clear() keeps the selection empty across later transactions", async () => {
    const get = renderHarness()
    await waitFor(() => expect(get().editor).toBeTruthy())
    const editor = get().editor!
    get().selectBlock(dividerPos(editor))
    await waitFor(() => expect(get().selectedType).toBe("divider"))
    act(() => get().clear())
    await waitFor(() => expect(get().selectedPos).toBeNull())
    // A programmatic edit (it moves the internal selection) must not
    // resurrect a block selection.
    act(() => {
      editor.commands.insertContentAt(editor.state.doc.content.size, { type: "paragraph" })
    })
    expect(get().selectedPos).toBeNull()
  })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/hooks/useBlockSelection.test.tsx`
Expected: FAIL. `"reports nothing selected…"` gets `selectedType` `"paragraph"`, and the `clear()` test sees a non-null position.

- [ ] **Step 3: Implement**

Replace the body of `useBlockSelection.ts` from `export function useBlockSelection` to the end of the file with:

```ts
function resolveSelection(editor: Editor): { pos: number | null; node: PMNode | null } {
  const pos = deriveSelectedPos(editor.state)
  return { pos, node: pos == null ? null : editor.state.doc.nodeAt(pos) }
}

export function useBlockSelection(editor: Editor | null): BlockSelection {
  const [state, setState] = useState<{ pos: number | null; node: PMNode | null }>(
    { pos: null, node: null },
  )
  // Whether the operator has picked a block. False on load and after clear():
  // ProseMirror always holds *some* selection internally, but until the user
  // clicks or types in the canvas (or picks a layer) nothing is "selected" and
  // the root «Email» node is current.
  const activeRef = useRef(false)

  useEffect(() => {
    if (!editor) {
      activeRef.current = false
      setState({ pos: null, node: null })
      return
    }
    const sync = () => {
      setState(activeRef.current ? resolveSelection(editor) : { pos: null, node: null })
    }
    const activate = () => {
      activeRef.current = true
      sync()
    }
    const dom = editor.view.dom
    sync()
    dom.addEventListener("mousedown", activate)
    dom.addEventListener("keydown", activate)
    editor.on("focus", activate)
    editor.on("selectionUpdate", sync)
    editor.on("transaction", sync)
    return () => {
      dom.removeEventListener("mousedown", activate)
      dom.removeEventListener("keydown", activate)
      editor.off("focus", activate)
      editor.off("selectionUpdate", sync)
      editor.off("transaction", sync)
    }
  }, [editor])

  const selectBlock = useCallback(
    (pos: number) => {
      if (!editor) return
      const node = editor.state.doc.nodeAt(pos)
      if (!node) return
      activeRef.current = true
      const chain = editor.chain().focus()
      if (node.isTextblock) {
        // Place the caret just inside the text block.
        chain.setTextSelection(pos + 1)
      } else {
        chain.setNodeSelection(pos)
      }
      chain.scrollIntoView().run()
      // Re-sync even if the selection did not move (no selectionUpdate fires).
      setState(resolveSelection(editor))
    },
    [editor],
  )

  const updateSelectedAttrs = useCallback(
    (attrs: Record<string, unknown>) => {
      if (!editor || state.pos == null) return
      const pos = state.pos
      editor
        .chain()
        .focus()
        .command(({ tr }) => {
          const node = tr.doc.nodeAt(pos)
          if (!node) return false
          tr.setNodeMarkup(pos, undefined, { ...node.attrs, ...attrs })
          return true
        })
        .run()
    },
    [editor, state.pos],
  )

  const clear = useCallback(() => {
    activeRef.current = false
    setState({ pos: null, node: null })
    editor?.commands.blur()
  }, [editor])

  return {
    selectedPos: state.pos,
    selectedNode: state.node,
    selectBlock,
    updateSelectedAttrs,
    clear,
  }
}
```

Change the React import to `import { useCallback, useEffect, useRef, useState } from "react"`. In the header comment, add one line saying that selection starts empty and `clear()` sticks until the next user intent.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/hooks src/components/visual-editor/panels/StructureOutline.test.tsx`
Expected: `EXIT:0`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/visual-editor/hooks/useBlockSelection.ts frontend/src/components/visual-editor/hooks/useBlockSelection.test.tsx
git commit -m "feat(visual-editor): start block selection empty and keep clear() sticky"
```

---

### Task 3: Block catalog, `insertBlock`, and the slash menu on top of it

**Files:**
- Create: `frontend/src/components/visual-editor/blockCatalog.ts`
- Create: `frontend/src/components/visual-editor/blockCatalog.test.ts`
- Modify: `frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx`
- Create: `frontend/src/components/visual-editor/ui/SlashCommandMenu.test.ts`

**Interfaces:**
- Produces:
  - `type CatalogType = "text" | "heading" | "image" | "button" | "columns" | "divider" | "html"`
  - `type CatalogKey` (union of the 15 item keys below)
  - `type CatalogItem = { key: CatalogKey; type: CatalogType; content: () => JSONContent }`
  - `const CATALOG_TYPES: ReadonlyArray<CatalogType>`, `const BLOCK_CATALOG: ReadonlyArray<CatalogItem>`
  - `insertBlock(editor: Editor, selectedPos: number | null, content: JSONContent): number`. Returns the new block's position.
  - `slashItems(query: string): Array<SlashItem>` exported from `SlashCommandMenu.tsx`.

- [ ] **Step 1: Write the failing catalog tests**

Create `frontend/src/components/visual-editor/blockCatalog.test.ts`:

```ts
// Tests for the shared block catalog and insertBlock(). The catalog feeds both
// the block library and the slash menu, so every entry needs labels in both
// locales and must build schema-valid content.

import { afterEach, beforeAll, describe, expect, it } from "vitest"
import i18n from "i18next"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import { BLOCK_CATALOG, CATALOG_TYPES, insertBlock } from "./blockCatalog"
import { Column, Columns } from "./extensions/Columns"
import { Divider } from "./extensions/Divider"
import type { JSONContent } from "@tiptap/core"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

const editors: Array<Editor> = []
afterEach(() => {
  editors.splice(0).forEach((e) => e.destroy())
})

function makeEditor(content: JSONContent): Editor {
  const editor = new Editor({
    extensions: [StarterKit.configure({ hardBreak: false }), Columns, Column, Divider],
    content,
  })
  editors.push(editor)
  return editor
}

const para = (text: string): JSONContent => ({ type: "paragraph", content: [{ type: "text", text }] })

function childTypes(editor: Editor): Array<string> {
  const out: Array<string> = []
  editor.state.doc.forEach((n) => out.push(n.type.name))
  return out
}

describe("BLOCK_CATALOG", () => {
  it("has unique keys and only declared types", () => {
    const keys = BLOCK_CATALOG.map((i) => i.key)
    expect(new Set(keys).size).toBe(keys.length)
    for (const item of BLOCK_CATALOG) expect(CATALOG_TYPES).toContain(item.type)
  })

  it("has a label for every item and type in en and ru", () => {
    for (const lng of ["en", "ru"]) {
      for (const item of BLOCK_CATALOG) {
        expect(i18n.exists(`visualEditor:catalog.item.${item.key}`, { lng })).toBe(true)
      }
      for (const type of CATALOG_TYPES) {
        expect(i18n.exists(`visualEditor:catalog.type.${type}`, { lng })).toBe(true)
      }
    }
  })

  it("builds columns whose content length matches the count", () => {
    for (const item of BLOCK_CATALOG.filter((i) => i.type === "columns")) {
      const node = item.content()
      expect(node.content).toHaveLength(node.attrs?.count as number)
    }
  })
})

describe("insertBlock", () => {
  it("inserts after the top-level block containing the selection", () => {
    const editor = makeEditor({ type: "doc", content: [para("AAA"), para("BBB")] })
    const at = insertBlock(editor, 0, { type: "divider" })
    expect(childTypes(editor).slice(0, 3)).toEqual(["paragraph", "divider", "paragraph"])
    expect(editor.state.doc.nodeAt(at)?.type.name).toBe("divider")
  })

  it("inserts after the enclosing top-level block when the selection is nested", () => {
    const editor = makeEditor({
      type: "doc",
      content: [
        {
          type: "columns",
          attrs: { count: 2 },
          content: [
            { type: "column", content: [para("left")] },
            { type: "column", content: [para("right")] },
          ],
        },
        para("after"),
      ],
    })
    let nested = -1
    editor.state.doc.descendants((n, p) => {
      if (nested < 0 && n.type.name === "paragraph") nested = p
      return true
    })
    insertBlock(editor, nested, { type: "divider" })
    expect(childTypes(editor).slice(0, 3)).toEqual(["columns", "divider", "paragraph"])
  })

  it("appends at the end when nothing is selected", () => {
    const editor = makeEditor({ type: "doc", content: [para("AAA")] })
    const at = insertBlock(editor, null, { type: "divider" })
    expect(editor.state.doc.nodeAt(at)?.type.name).toBe("divider")
    expect(childTypes(editor)[0]).toBe("paragraph")
  })

  it("replaces an empty document instead of appending", () => {
    const editor = makeEditor({ type: "doc", content: [{ type: "paragraph" }] })
    const at = insertBlock(editor, null, { type: "heading", attrs: { level: 1 } })
    expect(at).toBe(0)
    expect(childTypes(editor)[0]).toBe("heading")
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/blockCatalog.test.ts`
Expected: FAIL. `Cannot find module './blockCatalog'`.

- [ ] **Step 3: Implement the catalog**

Create `frontend/src/components/visual-editor/blockCatalog.ts`:

```ts
// blockCatalog — the single list of blocks an operator can insert. The block
// library (panels/BlockLibrary) and the slash menu (ui/SlashCommandMenu) both
// read it, so a block added here shows up in both places with the same i18n
// label (`visualEditor:catalog.item.<key>`). insertBlock() is the library's
// insertion rule: always a new top-level block.

import { buildColumnsNode } from "./extensions/Columns"
import type { Editor, JSONContent } from "@tiptap/core"

export type CatalogType =
  | "text"
  | "heading"
  | "image"
  | "button"
  | "columns"
  | "divider"
  | "html"

export type CatalogKey =
  | "paragraph"
  | "bullet-list"
  | "ordered-list"
  | "quote"
  | "code"
  | "heading-1"
  | "heading-2"
  | "heading-3"
  | "image"
  | "button"
  | "columns-2"
  | "columns-3"
  | "columns-4"
  | "divider"
  | "raw-html"

export type CatalogItem = {
  key: CatalogKey
  type: CatalogType
  content: () => JSONContent
}

export const CATALOG_TYPES: ReadonlyArray<CatalogType> = [
  "text",
  "heading",
  "image",
  "button",
  "columns",
  "divider",
  "html",
]

const emptyList = (type: "bulletList" | "orderedList"): JSONContent => ({
  type,
  content: [{ type: "listItem", content: [{ type: "paragraph" }] }],
})

export const BLOCK_CATALOG: ReadonlyArray<CatalogItem> = [
  { key: "paragraph", type: "text", content: () => ({ type: "paragraph" }) },
  { key: "bullet-list", type: "text", content: () => emptyList("bulletList") },
  { key: "ordered-list", type: "text", content: () => emptyList("orderedList") },
  {
    key: "quote",
    type: "text",
    content: () => ({ type: "blockquote", content: [{ type: "paragraph" }] }),
  },
  { key: "code", type: "text", content: () => ({ type: "codeBlock" }) },
  { key: "heading-1", type: "heading", content: () => ({ type: "heading", attrs: { level: 1 } }) },
  { key: "heading-2", type: "heading", content: () => ({ type: "heading", attrs: { level: 2 } }) },
  { key: "heading-3", type: "heading", content: () => ({ type: "heading", attrs: { level: 3 } }) },
  {
    key: "image",
    type: "image",
    content: () => ({ type: "image", attrs: { mediaRef: "", alt: "", href: "" } }),
  },
  {
    key: "button",
    type: "button",
    content: () => ({ type: "button", attrs: { label: "Button", href: "" } }),
  },
  { key: "columns-2", type: "columns", content: () => buildColumnsNode(2) },
  { key: "columns-3", type: "columns", content: () => buildColumnsNode(3) },
  { key: "columns-4", type: "columns", content: () => buildColumnsNode(4) },
  { key: "divider", type: "divider", content: () => ({ type: "divider" }) },
  { key: "raw-html", type: "html", content: () => ({ type: "rawHtml", attrs: { html: "" } }) },
]

// insertBlock inserts `content` as a new top-level block right after the
// top-level block that contains `selectedPos`, or at the end of the document
// when nothing is selected. An empty document (a single empty paragraph) is
// replaced rather than appended to. Returns the new block's position.
export function insertBlock(
  editor: Editor,
  selectedPos: number | null,
  content: JSONContent,
): number {
  const { doc } = editor.state
  if (editor.isEmpty) {
    editor.chain().insertContentAt({ from: 0, to: doc.content.size }, content).run()
    return 0
  }
  let at = doc.content.size
  if (selectedPos != null) {
    const $pos = doc.resolve(selectedPos)
    const top = $pos.depth === 0 ? selectedPos : $pos.before(1)
    const topNode = doc.nodeAt(top)
    if (topNode) at = top + topNode.nodeSize
  }
  editor.chain().insertContentAt(at, content).run()
  return at
}
```

- [ ] **Step 4: Run the catalog tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/blockCatalog.test.ts`
Expected: `EXIT:0`.

- [ ] **Step 5: Write the failing slash-menu test**

Create `frontend/src/components/visual-editor/ui/SlashCommandMenu.test.ts`:

```ts
// slashItems() builds the slash menu from the shared catalog with labels from
// the active i18n language (setup.ts initializes English).

import { describe, expect, it } from "vitest"
import { BLOCK_CATALOG } from "../blockCatalog"
import { slashItems } from "./SlashCommandMenu"

describe("slashItems", () => {
  it("lists every catalog block plus the merge tag, with translated labels", () => {
    const items = slashItems("")
    expect(items.map((i) => i.key)).toEqual([...BLOCK_CATALOG.map((i) => i.key), "merge-tag"])
    expect(items.find((i) => i.key === "heading-1")?.label).toBe("Heading 1")
    expect(items.find((i) => i.key === "merge-tag")?.description).toBe(
      "Insert a subscriber or campaign placeholder",
    )
  })

  it("filters by the typed query, case-insensitively", () => {
    expect(slashItems("HEAD").map((i) => i.key)).toEqual(["heading-1", "heading-2", "heading-3"])
  })
})
```

- [ ] **Step 6: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/ui/SlashCommandMenu.test.ts`
Expected: FAIL. `slashItems` is not exported.

- [ ] **Step 7: Rebuild the slash items from the catalog**

In `frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx`:

1. Replace `import { buildColumnsNode } from "../extensions/Columns"` with:
```ts
import i18n from "i18next"
import { BLOCK_CATALOG } from "../blockCatalog"
```
2. Delete the whole `const SLASH_ITEMS: Array<SlashItem> = [ ... ]` array and put this function in its place:

```ts
// slashItems builds the menu from the shared block catalog (plus the
// slash-only merge-tag entry), labelled in the active i18n language, then
// filters by the typed query. Applying an item inserts the catalog content at
// the slash position. On an empty line TipTap replaces the empty paragraph,
// matching the old setNode/toggle behaviour.
export function slashItems(query: string): Array<SlashItem> {
  const items: Array<SlashItem> = BLOCK_CATALOG.map((item) => ({
    key: item.key,
    label: i18n.t(`visualEditor:catalog.item.${item.key}`),
    apply: (editor, range) => {
      editor.chain().focus().deleteRange(range).insertContent(item.content()).run()
    },
  }))
  items.push({
    key: "merge-tag",
    label: i18n.t("visualEditor:catalog.item.merge-tag"),
    description: i18n.t("visualEditor:catalog.mergeTagDescription"),
    apply: (editor, range) => {
      editor.chain().focus().deleteRange(range).run()
      // The MergeTagPicker UI listens for this event and opens, then
      // dispatches an insert command into the editor.
      editor.view.dom.dispatchEvent(
        new CustomEvent("visual-editor:merge-tag-open", { bubbles: true }),
      )
    },
  })
  const q = query.toLowerCase()
  return items.filter((i) => i.label.toLowerCase().includes(q))
}
```

3. In `SlashCommandExtension`'s `Suggestion({...})`, replace the `items: ({ query }) => SLASH_ITEMS.filter(...)` property with `items: ({ query }) => slashItems(query),`.

If `tsc` rejects the `"visualEditor:…"` key form, use `i18n.t(\`catalog.item.${item.key}\`, { ns: "visualEditor" })` instead (same for the other two calls).

- [ ] **Step 8: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor`
Expected: `EXIT:0` (includes `VisualEmailEditor.test.tsx` and the slash story).

- [ ] **Step 9: Typecheck, lint, commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/blockCatalog.ts src/components/visual-editor/blockCatalog.test.ts src/components/visual-editor/ui/SlashCommandMenu.tsx src/components/visual-editor/ui/SlashCommandMenu.test.ts && pnpm typecheck && cd ..
git add frontend/src/components/visual-editor/blockCatalog.ts frontend/src/components/visual-editor/blockCatalog.test.ts frontend/src/components/visual-editor/ui/SlashCommandMenu.tsx frontend/src/components/visual-editor/ui/SlashCommandMenu.test.ts
git commit -m "feat(visual-editor): shared block catalog for the library and slash menu"
```

---

### Task 4: `setColumnsCount` — change the column count without losing content

**Files:**
- Modify: `frontend/src/components/visual-editor/extensions/Columns.tsx`
- Create: `frontend/src/components/visual-editor/extensions/Columns.test.ts`

**Interfaces:**
- Produces: `setColumnsCount(editor: Editor, pos: number, count: ColumnsCount): void`. Afterwards the columns node is node-selected at `pos`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/visual-editor/extensions/Columns.test.ts`:

```ts
import { afterEach, beforeAll, describe, expect, it } from "vitest"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import { Column, Columns, buildColumnsNode, setColumnsCount } from "./Columns"
import type { JSONContent } from "@tiptap/core"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

let editor: Editor | null = null
afterEach(() => editor?.destroy())

const para = (text: string): JSONContent => ({ type: "paragraph", content: [{ type: "text", text }] })

function columnsDoc(texts: Array<string>): JSONContent {
  return {
    type: "doc",
    content: [
      {
        type: "columns",
        attrs: { count: texts.length },
        content: texts.map((t) => ({ type: "column", content: [para(t)] })),
      },
    ],
  }
}

function make(content: JSONContent): Editor {
  editor = new Editor({ extensions: [StarterKit.configure({ hardBreak: false }), Columns, Column], content })
  return editor
}

describe("setColumnsCount", () => {
  it("adds empty columns when growing", () => {
    const ed = make(columnsDoc(["a", "b"]))
    setColumnsCount(ed, 0, 3)
    const node = ed.state.doc.child(0)
    expect(node.attrs.count).toBe(3)
    expect(node.childCount).toBe(3)
    expect(node.child(2).textContent).toBe("")
  })

  it("folds removed columns into the last kept column when shrinking", () => {
    const ed = make(columnsDoc(["a", "b", "c", "d"]))
    setColumnsCount(ed, 0, 2)
    const node = ed.state.doc.child(0)
    expect(node.attrs.count).toBe(2)
    expect(node.childCount).toBe(2)
    expect(node.child(0).textContent).toBe("a")
    expect(node.child(1).textContent).toBe("bcd")
  })

  it("is a no-op for the same count or a non-columns node", () => {
    const ed = make(columnsDoc(["a", "b"]))
    const before = ed.getJSON()
    setColumnsCount(ed, 0, 2)
    setColumnsCount(ed, 1, 3)
    expect(ed.getJSON()).toEqual(before)
  })

  it("keeps buildColumnsNode's count invariant", () => {
    expect(buildColumnsNode(3).content).toHaveLength(3)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/extensions/Columns.test.ts`
Expected: FAIL. `setColumnsCount` is not exported.

- [ ] **Step 3: Implement**

Append to `frontend/src/components/visual-editor/extensions/Columns.tsx`:

```ts
// setColumnsCount changes a columns block to `count` columns in one
// transaction, keeping `attrs.count === content length` (validation
// invariant). Growing appends empty columns. Shrinking folds the removed
// columns' content into the last kept column, so nothing is lost. The columns
// block stays node-selected so the parameters panel keeps showing it.
export function setColumnsCount(editor: Editor, pos: number, count: ColumnsCount): void {
  const node = editor.state.doc.nodeAt(pos)
  if (!node || node.type.name !== "columns" || node.childCount === count) return
  const { schema } = editor
  const columns: Array<PMNode> = []
  node.forEach((child) => columns.push(child))

  let next: Array<PMNode>
  if (count > columns.length) {
    const added = Array.from({ length: count - columns.length }, () =>
      schema.nodes.column.create(null, schema.nodes.paragraph.create()),
    )
    next = [...columns, ...added]
  } else {
    const kept = columns.slice(0, count)
    const last = kept[count - 1]
    const merged = columns
      .slice(count)
      .reduce((acc, col) => acc.append(col.content), last.content)
    next = [...kept.slice(0, count - 1), last.copy(merged)]
  }

  const replacement = node.type.create({ ...node.attrs, count }, next)
  editor
    .chain()
    .focus()
    .command(({ tr }) => {
      tr.replaceWith(pos, pos + node.nodeSize, replacement)
      tr.setSelection(NodeSelection.create(tr.doc, pos))
      return true
    })
    .run()
}
```

Update the imports at the top of `Columns.tsx`:

```ts
import { Node, mergeAttributes } from "@tiptap/core"
import { NodeSelection } from "@tiptap/pm/state"
import { blockStyleAttributeSpec } from "./styleAttr"
import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
```

- [ ] **Step 4: Run the test**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/extensions/Columns.test.ts`
Expected: `EXIT:0`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/visual-editor/extensions/Columns.tsx frontend/src/components/visual-editor/extensions/Columns.test.ts
git commit -m "feat(visual-editor): change a columns block's count without losing content"
```

---

### Task 5: Design-tab look — grouped style controls with switches

**Files:**
- Modify: `frontend/src/components/visual-editor/panels/params/StyleControls.tsx`
- Create: `frontend/src/components/visual-editor/panels/params/StyleControls.test.tsx`
- Modify: `frontend/src/components/visual-editor/panels/params/StyleControls.stories.tsx`, `ButtonParams.stories.tsx`, `ColumnsParams.stories.tsx` (play functions)
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Produces (all from `StyleControls.tsx`):
  - `type ParamsTab = "content" | "design"`
  - `ParamFormProps` extended with `tab?: ParamsTab`, `editor?: Editor | null`, `pos?: number`
  - Test ids: `ve-style-section-{typography|spacing|background|border}`, `ve-param-border-toggle`, `ve-param-borderRadius-toggle`. Existing ids (`ve-param-borderRadius`, `ve-param-paddingTop`, …) are kept.

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/components/visual-editor/panels/params/StyleControls.test.tsx`:

```tsx
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { StyleControls } from "./StyleControls"

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("StyleControls", () => {
  it("renders only the sections that have fields", () => {
    render(<StyleControls value={{}} fields={["color", "padding"]} onChange={vi.fn()} />)
    expect(screen.getByTestId("ve-style-section-typography")).toBeTruthy()
    expect(screen.getByTestId("ve-style-section-spacing")).toBeTruthy()
    expect(screen.queryByTestId("ve-style-section-background")).toBeNull()
    expect(screen.queryByTestId("ve-style-section-border")).toBeNull()
  })

  it("hides border fields behind an off switch until it is turned on", () => {
    render(<StyleControls value={{}} fields={["border"]} onChange={vi.fn()} />)
    const toggle = screen.getByTestId("ve-param-border-toggle") as HTMLInputElement
    expect(toggle.checked).toBe(false)
    expect(screen.queryByTestId("ve-param-borderWidth")).toBeNull()
    fireEvent.click(toggle)
    expect(screen.getByTestId("ve-param-borderWidth")).toBeTruthy()
  })

  it("turning the border switch off clears all border fields", () => {
    const onChange = vi.fn()
    render(
      <StyleControls
        value={{ borderWidth: 2, borderStyle: "solid", borderColor: "#000000" }}
        fields={["border"]}
        onChange={onChange}
      />,
    )
    const toggle = screen.getByTestId("ve-param-border-toggle") as HTMLInputElement
    expect(toggle.checked).toBe(true)
    fireEvent.click(toggle)
    expect(onChange).toHaveBeenCalledWith({
      borderWidth: undefined,
      borderStyle: undefined,
      borderColor: undefined,
    })
  })

  it("puts corner radius behind its own switch", () => {
    const onChange = vi.fn()
    render(<StyleControls value={{ borderRadius: 12 }} fields={["borderRadius"]} onChange={onChange} />)
    const toggle = screen.getByTestId("ve-param-borderRadius-toggle") as HTMLInputElement
    expect(toggle.checked).toBe(true)
    expect((screen.getByTestId("ve-param-borderRadius") as HTMLInputElement).value).toBe("12")
    fireEvent.click(toggle)
    expect(onChange).toHaveBeenCalledWith({ borderRadius: undefined })
  })

  it("lays padding out left, top, bottom, right", () => {
    const { container } = render(<StyleControls value={{}} fields={["padding"]} onChange={vi.fn()} />)
    const ids = Array.from(container.querySelectorAll("[data-testid^='ve-param-padding']")).map((el) =>
      el.getAttribute("data-testid"),
    )
    expect(ids).toEqual([
      "ve-param-paddingLeft",
      "ve-param-paddingTop",
      "ve-param-paddingBottom",
      "ve-param-paddingRight",
    ])
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels/params/StyleControls.test.tsx`
Expected: FAIL. No `ve-style-section-*` and no toggles.

- [ ] **Step 3: Implement**

In `StyleControls.tsx`:

1. Update the header comment: fields are grouped into titled sections, and border/radius sit behind switches whose "off" clears the fields.
2. Imports: add `import { AlignCenter, AlignLeft, AlignRight } from "lucide-react"` and `import type { Editor } from "@tiptap/core"`.
3. Add after the `StyleField` type:
```ts
export type ParamsTab = "content" | "design"
```
4. Replace `ParamFormProps` with:
```ts
export type ParamFormProps = {
  blockType: string
  attrs: Record<string, unknown>
  style: BlockStyle
  onStyleChange: (patch: Partial<BlockStyle>) => void
  onAttrsChange: (patch: Record<string, unknown>) => void
  disabled?: boolean
  // Settings tab being shown; a form renders only that tab's fields. Omitted
  // (stories, standalone use) means both tabs' fields.
  tab?: ParamsTab
  // For actions that are more than an attr patch (replace image, edit raw
  // HTML, change the column count). Null/omitted disables those actions.
  editor?: Editor | null
  pos?: number
}
```
5. Replace the `StyleControls` function with:
```tsx
type SectionKey = "typography" | "spacing" | "background" | "border"

const SECTIONS: ReadonlyArray<{ key: SectionKey; fields: ReadonlyArray<StyleField> }> = [
  { key: "typography", fields: ["color", "fontFamily", "fontSize", "fontWeight", "lineHeight"] },
  { key: "spacing", fields: ["padding", "textAlign"] },
  { key: "background", fields: ["backgroundColor"] },
  { key: "border", fields: ["border", "borderRadius"] },
]

export function StyleControls({ value, fields, onChange, disabled = false }: Props) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-style-controls">
      {SECTIONS.map((section) => {
        const present = section.fields.filter((f) => fields.includes(f))
        if (present.length === 0) return null
        return (
          <section
            key={section.key}
            className="ve-style-controls__section"
            data-testid={`ve-style-section-${section.key}`}
          >
            <h4 className="ve-style-controls__heading">{t(`params.sections.${section.key}`)}</h4>
            {present.map((field) => (
              <Field key={field} field={field} value={value} onChange={onChange} disabled={disabled} t={t} />
            ))}
          </section>
        )
      })}
    </div>
  )
}
```
6. In `Field`, replace the `case "borderRadius":` branch with:
```tsx
    case "borderRadius":
      return (
        <RadiusField
          value={value.borderRadius}
          onChange={(v) => onChange({ borderRadius: v })}
          disabled={disabled}
          t={t}
        />
      )
```
7. Add a `Switch` and a `RadiusField` (place them after `ResetButton`):
```tsx
// Switch is a native checkbox styled as a toggle (role="switch"), with no
// extra dependency and fully keyboard/screen-reader accessible.
function Switch({
  label,
  testId,
  checked,
  onChange,
  disabled,
}: {
  label: string
  testId: string
  checked: boolean
  onChange: (next: boolean) => void
  disabled: boolean
}) {
  return (
    <label className="ve-style-controls__toggle">
      <input
        type="checkbox"
        role="switch"
        data-testid={testId}
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        disabled={disabled}
      />
      <span>{label}</span>
    </label>
  )
}

function RadiusField({
  value,
  onChange,
  disabled,
  t,
}: {
  value?: number
  onChange: (v: number | undefined) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  const [open, setOpen] = useState(value !== undefined)
  const on = open || value !== undefined
  return (
    <div className="ve-style-controls__group">
      <Switch
        label={t("params.toggle.radius")}
        testId="ve-param-borderRadius-toggle"
        checked={on}
        disabled={disabled}
        onChange={(next) => {
          setOpen(next)
          if (!next) onChange(undefined)
        }}
      />
      {on && (
        <NumberField
          label={t("params.fields.borderRadius")}
          testId="ve-param-borderRadius"
          min={0}
          max={48}
          value={value}
          onChange={onChange}
          disabled={disabled}
          t={t}
        />
      )}
    </div>
  )
}
```
8. Give `NumberField` a compact variant. Add `compact = false` to its destructured props and `compact?: boolean` to its prop type. Hoist the `<input type="number" … />` element into a `const input = (...)` (same attributes as today), then:
```tsx
  if (compact) {
    // Compact (padding row): label above, no reset button. Clearing the
    // input is the reset.
    return (
      <label className="ve-style-controls__compact">
        <span>{label}</span>
        {input}
      </label>
    )
  }
```
The non-compact return stays as today: label, reset slot, then `{input}`.
9. Replace `PaddingGroup`'s body with:
```tsx
  // Left / top / bottom / right — the order of the reference layout.
  const sides = [
    ["paddingLeft", "params.fields.paddingLeft"],
    ["paddingTop", "params.fields.paddingTop"],
    ["paddingBottom", "params.fields.paddingBottom"],
    ["paddingRight", "params.fields.paddingRight"],
  ] as const
  return (
    <fieldset className="ve-style-controls__group">
      <legend>{t("params.fields.padding")}</legend>
      <div className="ve-style-controls__padding">
        {sides.map(([key, label]) => (
          <NumberField
            key={key}
            compact
            label={t(label)}
            testId={`ve-param-${key}`}
            min={0}
            max={64}
            value={value[key]}
            onChange={(v) => onChange({ [key]: v })}
            disabled={disabled}
            t={t}
          />
        ))}
      </div>
    </fieldset>
  )
```
10. Replace `BorderGroup`'s body with:
```tsx
  const set =
    value.borderWidth !== undefined || value.borderStyle !== undefined || value.borderColor !== undefined
  const [open, setOpen] = useState(set)
  const on = open || set
  return (
    <div className="ve-style-controls__group">
      <Switch
        label={t("params.toggle.border")}
        testId="ve-param-border-toggle"
        checked={on}
        disabled={disabled}
        onChange={(next) => {
          setOpen(next)
          if (!next) onChange({ borderWidth: undefined, borderStyle: undefined, borderColor: undefined })
        }}
      />
      {on && (
        <>
          {/* the existing NumberField (borderWidth), SelectField (borderStyle)
              and ColorField (borderColor), unchanged */}
        </>
      )}
    </div>
  )
```
Move the three existing child controls (`ve-param-borderWidth`, `ve-param-borderStyle`, `ve-param-borderColor`) verbatim into that fragment, replacing the comment.
11. In `AlignField`, render icons instead of text:
```tsx
const ALIGN_ICONS = { left: AlignLeft, center: AlignCenter, right: AlignRight } as const
```
(module scope), and inside the `options.map`:
```tsx
        {options.map((opt) => {
          const Icon = ALIGN_ICONS[opt]
          return (
            <button
              key={opt}
              type="button"
              data-testid={`ve-param-align-${opt}`}
              aria-pressed={value === opt}
              aria-label={t(`params.align.${opt}`)}
              title={t(`params.align.${opt}`)}
              className={value === opt ? "is-active" : ""}
              onClick={() => onChange(value === opt ? undefined : opt)}
              disabled={disabled}
            >
              <Icon className="size-4" aria-hidden />
            </button>
          )
        })}
```

- [ ] **Step 4: CSS**

In `visual-editor.css`, replace the `.ve-style-controls__align`, `.ve-style-controls__align button` and `.ve-style-controls__align button.is-active` rules with:

```css
.ve-style-controls__align {
  display: inline-flex;
  padding: 2px;
  border-radius: 6px;
  background: #e4e4e7;
}

.ve-style-controls__align button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  padding: 4px 8px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: #3f3f46;
  cursor: pointer;
}

.ve-style-controls__align button.is-active {
  background: #fff;
  color: #111827;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}
```

and append:

```css
/* Grouped sections (Design tab) — editor shell 2026-11-14. */
.ve-style-controls__section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 0;
  border-bottom: 1px solid #e5e7eb;
}

.ve-style-controls__section:last-child {
  border-bottom: none;
}

.ve-style-controls__heading {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: #111827;
}

.ve-style-controls__padding {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 6px;
}

.ve-style-controls__compact {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11px;
  color: #6b7280;
}

.ve-style-controls__compact input {
  width: 100%;
  max-width: none;
}

.ve-style-controls__toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  cursor: pointer;
}

.ve-style-controls__toggle input[role="switch"] {
  appearance: none;
  position: relative;
  width: 32px;
  height: 18px;
  margin: 0;
  border-radius: 9px;
  background: #d4d4d8;
  cursor: pointer;
  transition: background 0.15s;
}

.ve-style-controls__toggle input[role="switch"]::before {
  content: "";
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.15s;
}

.ve-style-controls__toggle input[role="switch"]:checked {
  background: #3f3f46;
}

.ve-style-controls__toggle input[role="switch"]:checked::before {
  transform: translateX(14px);
}
```

- [ ] **Step 5: Update story play functions for the radius switch**

In `StyleControls.stories.tsx`, `ButtonParams.stories.tsx` and `ColumnsParams.stories.tsx`, find each play function that types into `ve-param-borderRadius`. Directly before that `userEvent.type(...)` line, add:
```ts
    await userEvent.click(canvas.getByTestId("ve-param-borderRadius-toggle"))
```
Skip this where the story's initial style already sets `borderRadius`: the switch starts on there, and clicking it would turn it off.

- [ ] **Step 6: Keep the existing panel tests green**

In `BlockParamsPanel.test.tsx`, the tests `"merges a new field over the existing style"` and `"constrains numeric controls to their email-safe bounds"` touch `ve-param-borderRadius`, which now sits behind its switch. In each, add `fireEvent.click(screen.getByTestId("ve-param-borderRadius-toggle"))` right after `render(...)`. (Task 6 rewrites this file anyway; this keeps the commit green.)

- [ ] **Step 7: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels`
Expected: `EXIT:0`.

- [ ] **Step 8: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/panels/params && cd ..
git add frontend/src/components/visual-editor/panels/params frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): group style controls into sections with border/radius switches"
```

---

### Task 6: Content / Design tabs in the parameters panel

**Files:**
- Modify: `frontend/src/components/visual-editor/panels/BlockParamsPanel.tsx`
- Modify: `frontend/src/components/visual-editor/panels/params/ButtonParams.tsx`, `TextParams.tsx`, `ImageParams.tsx`, `ColumnsParams.tsx`
- Create: `frontend/src/components/visual-editor/panels/params/RawHtmlParams.tsx`
- Test: `frontend/src/components/visual-editor/panels/BlockParamsPanel.test.tsx` (rewrite)
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Consumes: `ParamFormProps`, `ParamsTab` (Task 5); `setColumnsCount` (Task 4); `IMAGEBLOCK_PICK_EVENT`, `ImageBlockPickRequest` from `extensions/ImageBlock`; `RAWHTML_EDIT_EVENT`, `RawHTMLEditRequest` from `extensions/RawHTML`.
- Produces:
  - `BlockParamsPanel({ selection, editor?, root?, disabled? })`. `root` is rendered on the Design tab when nothing is selected.
  - `tabsFor(blockType: string | null): Array<ParamsTab>`
  - Test ids: `ve-params-tab-content`, `ve-params-tab-design`, `ve-param-replace-image`, `ve-param-edit-html`, `ve-param-columns-{2,3,4}` (inside `ve-param-column-count`).

- [ ] **Step 1: Rewrite the panel tests (failing)**

Replace `frontend/src/components/visual-editor/panels/BlockParamsPanel.test.tsx` with:

```tsx
// Tests for BlockParamsPanel — Content/Design tabs, the root slot, live-apply
// through the shared selection, and the non-attr actions (replace image,
// change column count). Driven with a fabricated BlockSelection; Radix tabs
// switch on mousedown, so the tests use fireEvent.mouseDown on a trigger.

import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { IMAGEBLOCK_PICK_EVENT } from "../extensions/ImageBlock"
import { setColumnsCount } from "../extensions/Columns"
import { BlockParamsPanel, tabsFor } from "./BlockParamsPanel"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Editor } from "@tiptap/core"

vi.mock("../extensions/Columns", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../extensions/Columns")>()),
  setColumnsCount: vi.fn(),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

type FakeNode = { type: { name: string }; attrs: Record<string, unknown> }

function selectionFor(
  node: FakeNode | null,
): BlockSelection & { updateSelectedAttrs: ReturnType<typeof vi.fn> } {
  return {
    selectedPos: node ? 0 : null,
    selectedNode: node as unknown as BlockSelection["selectedNode"],
    selectBlock: vi.fn(),
    updateSelectedAttrs: vi.fn<(attrs: Record<string, unknown>) => void>(),
    clear: vi.fn(),
  }
}

const buttonNode = (attrs: Record<string, unknown> = {}): FakeNode => ({
  type: { name: "button" },
  attrs: { label: "Go", href: "https://example.test/x", ...attrs },
})

const openDesign = () => fireEvent.mouseDown(screen.getByTestId("ve-params-tab-design"))

describe("tabsFor", () => {
  it("maps each block type to its tabs", () => {
    expect(tabsFor(null)).toEqual(["design"])
    expect(tabsFor("button")).toEqual(["content", "design"])
    expect(tabsFor("heading")).toEqual(["content", "design"])
    expect(tabsFor("paragraph")).toEqual(["design"])
    expect(tabsFor("columns")).toEqual(["design"])
    expect(tabsFor("rawHtml")).toEqual(["content"])
    expect(tabsFor("listItem")).toEqual([])
  })
})

describe("BlockParamsPanel", () => {
  it("shows the root slot on the Design tab when nothing is selected", () => {
    render(<BlockParamsPanel selection={selectionFor(null)} root={<div data-testid="root-slot" />} />)
    expect(screen.getByTestId("root-slot")).toBeTruthy()
    expect(screen.getByTestId("ve-params-tab-design")).toBeTruthy()
    expect(screen.queryByTestId("ve-params-tab-content")).toBeNull()
  })

  it("falls back to the empty message without a root slot", () => {
    render(<BlockParamsPanel selection={selectionFor(null)} />)
    expect(screen.getByTestId("ve-params-empty")).toBeTruthy()
  })

  it("opens a button on Content and shows style controls on Design", () => {
    render(<BlockParamsPanel selection={selectionFor(buttonNode())} />)
    expect(screen.getByTestId("ve-param-label")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-backgroundColor")).toBeNull()
    openDesign()
    expect(screen.getByTestId("ve-param-backgroundColor")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-label")).toBeNull()
  })

  it("shows only Design with text controls (no background) for a paragraph", () => {
    render(<BlockParamsPanel selection={selectionFor({ type: { name: "paragraph" }, attrs: {} })} />)
    expect(screen.queryByTestId("ve-params-tab-content")).toBeNull()
    expect(screen.getByTestId("ve-param-fontSize")).toBeTruthy()
    expect(screen.queryByTestId("ve-param-backgroundColor")).toBeNull()
  })

  it("applies a style change to the selected block", () => {
    const sel = selectionFor(buttonNode())
    render(<BlockParamsPanel selection={sel} />)
    openDesign()
    fireEvent.change(screen.getByTestId("ve-param-backgroundColor"), { target: { value: "#ff0000" } })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ style: { backgroundColor: "#ff0000" } })
  })

  it("edits a type-specific attr without touching style", () => {
    const sel = selectionFor(buttonNode())
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.change(screen.getByTestId("ve-param-label"), { target: { value: "Read more" } })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ label: "Read more" })
  })

  it("merges a new field over the existing style", () => {
    const sel = selectionFor(buttonNode({ style: { backgroundColor: "#111111" } }))
    render(<BlockParamsPanel selection={sel} />)
    openDesign()
    fireEvent.click(screen.getByTestId("ve-param-borderRadius-toggle"))
    fireEvent.change(screen.getByTestId("ve-param-borderRadius"), { target: { value: "10" } })
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({
      style: { backgroundColor: "#111111", borderRadius: 10 },
    })
  })

  it("reset-all clears the block's style", () => {
    const sel = selectionFor(buttonNode({ style: { backgroundColor: "#111111" } }))
    render(<BlockParamsPanel selection={sel} />)
    fireEvent.click(screen.getByTestId("ve-params-reset-all"))
    expect(sel.updateSelectedAttrs).toHaveBeenCalledWith({ style: null })
  })

  it("constrains numeric controls to their email-safe bounds", () => {
    render(<BlockParamsPanel selection={selectionFor(buttonNode())} />)
    openDesign()
    fireEvent.click(screen.getByTestId("ve-param-borderRadius-toggle"))
    const radius = screen.getByTestId("ve-param-borderRadius") as HTMLInputElement
    expect(radius.type).toBe("number")
    expect(radius.min).toBe("0")
    expect(radius.max).toBe("48")
  })

  it("asks the canvas to open the media picker for the selected image", () => {
    const dom = document.createElement("div")
    const onPick = vi.fn()
    dom.addEventListener(IMAGEBLOCK_PICK_EVENT, (e) => onPick((e as CustomEvent).detail))
    const editor = { view: { dom } } as unknown as Editor
    const image: FakeNode = { type: { name: "image" }, attrs: { mediaRef: "https://cdn/x.png", alt: "Logo", href: "" } }
    render(<BlockParamsPanel selection={selectionFor(image)} editor={editor} />)
    fireEvent.click(screen.getByTestId("ve-param-replace-image"))
    expect(onPick).toHaveBeenCalledWith({ pos: 0, mediaRef: "https://cdn/x.png", alt: "Logo" })
  })

  it("changes the column count through setColumnsCount", () => {
    const editor = {} as Editor
    const columns: FakeNode = { type: { name: "columns" }, attrs: { count: 2 } }
    render(<BlockParamsPanel selection={selectionFor(columns)} editor={editor} />)
    expect(screen.getByTestId("ve-param-columns-2").getAttribute("aria-pressed")).toBe("true")
    fireEvent.click(screen.getByTestId("ve-param-columns-3"))
    expect(setColumnsCount).toHaveBeenCalledWith(editor, 0, 3)
  })

  it("says there are no properties for a list item", () => {
    render(<BlockParamsPanel selection={selectionFor({ type: { name: "listItem" }, attrs: {} })} />)
    expect(screen.getByTestId("ve-params-none")).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels/BlockParamsPanel.test.tsx`
Expected: FAIL. `tabsFor` is not exported, and there are no tab triggers.

- [ ] **Step 3: Split each form by tab**

`ButtonParams.tsx`: destructure `tab` too, and render:
```tsx
    <div className="ve-params-form" data-testid="ve-params-button">
      {tab !== "design" && (
        <>
          {/* the two existing TextInputs (label, href), unchanged */}
        </>
      )}
      {tab !== "content" && (
        <StyleControls value={style} fields={BUTTON_FIELDS} onChange={onStyleChange} disabled={disabled} />
      )}
    </div>
```
Move the two existing `TextInput` elements into the fragment in place of the comment.

`TextParams.tsx`: destructure `tab`. Wrap the existing heading-level `<label>…</label>` in `{tab !== "design" && blockType === "heading" && (…)}` and the `StyleControls` in `{tab !== "content" && (…)}`.

`ImageParams.tsx`: replace the component with:
```tsx
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
```
with imports:
```ts
import { IMAGEBLOCK_PICK_EVENT } from "../../extensions/ImageBlock"
import type { ImageBlockPickRequest } from "../../extensions/ImageBlock"
```

`ColumnsParams.tsx`: replace the component and update its header comment to "count is changed here via setColumnsCount":
```tsx
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
```
with `import { setColumnsCount } from "../../extensions/Columns"`. Columns and dividers only ever get the Design tab, so neither form needs to look at `tab`. `DividerParams.tsx` is unchanged.

Create `frontend/src/components/visual-editor/panels/params/RawHtmlParams.tsx`:
```tsx
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
```

- [ ] **Step 4: Rewrite `BlockParamsPanel.tsx`**

Replace the file with:
```tsx
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
```

- [ ] **Step 5: CSS**

Append to `visual-editor.css`:
```css
.ve-panel__tabs {
  gap: 8px;
}

.ve-params-action {
  align-self: flex-start;
  font-size: 12px;
  padding: 4px 10px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.ve-params-action:disabled {
  color: #9ca3af;
  cursor: default;
}
```

- [ ] **Step 6: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor`
Expected: `EXIT:0`. If the `ColumnsParams` story still asserts the old read-only count text, keep its `ve-param-column-count` assertion (the group now carries that id) and drop any text match.

- [ ] **Step 7: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/panels && pnpm typecheck && cd ..
git add frontend/src/components/visual-editor/panels frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): split block settings into Content and Design tabs"
```

---

### Task 7: Layers — root «Email» row, ⋮ menu, read-only mode

**Files:**
- Modify: `frontend/src/components/visual-editor/panels/StructureOutline.tsx`
- Test: `frontend/src/components/visual-editor/panels/StructureOutline.test.tsx`
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Consumes: `BlockSelection` (`clear()` is sticky, Task 2).
- Produces:
  - `StructureOutline({ editor, selection, editable? = true })`
  - `export function deleteBlock(editor: Editor, pos: number): void`
  - `export function duplicateBlock(editor: Editor, pos: number): void`
  - `export type BlockLabelInput = { type: string; level?: number; count?: number }`
  - `export function labelFor(entry: BlockLabelInput, t: TFunction<"visualEditor">): string`
  - Test ids: `ve-outline-root`, `ve-outline-menu-${pos}`; menu items keep `ve-outline-duplicate-${pos}` / `ve-outline-delete-${pos}`.

- [ ] **Step 1: Update and add tests (failing)**

In `StructureOutline.test.tsx`:

1. Change `Harness` to accept `editable` and pass it through:
```tsx
function Harness({
  content,
  onReady,
  editable = true,
}: {
  content: object
  onReady: (e: Editor) => void
  editable?: boolean
}) {
  // …existing useEditor / useBlockSelection / useEffect…
  return (
    <div>
      <StructureOutline editor={editor} selection={selection} editable={editable} />
      <EditorContent editor={editor} />
    </div>
  )
}

function renderHarness(content: object, editable = true) {
  let editor: Editor | null = null
  render(<Harness content={content} editable={editable} onReady={(e) => (editor = e)} />)
  return () => editor
}
```
2. Replace the test `"deletes and duplicates a block from the outline"` with:
```tsx
  it("deletes and duplicates a block from the row menu", async () => {
    const get = renderHarness(threeParas)
    await waitFor(() => expect(get()).toBeTruthy())
    const editor = get()!
    expect(editor.state.doc.childCount).toBe(3)
    const firstPos = topLevelPositions(editor)[0]
    fireEvent.keyDown(screen.getByTestId(`ve-outline-menu-${firstPos}`), { key: "Enter" })
    fireEvent.click(await screen.findByTestId(`ve-outline-duplicate-${firstPos}`))
    await waitFor(() => expect(editor.state.doc.childCount).toBe(4))
    const delPos = topLevelPositions(editor)[0]
    fireEvent.keyDown(screen.getByTestId(`ve-outline-menu-${delPos}`), { key: "Enter" })
    fireEvent.click(await screen.findByTestId(`ve-outline-delete-${delPos}`))
    await waitFor(() => expect(editor.state.doc.childCount).toBe(3))
  })
```
(If Radix does not open on `keyDown` Enter in jsdom, use `fireEvent.pointerDown(trigger, { button: 0, pointerType: "mouse" })`.)
3. Add:
```tsx
  it("shows a root «Email» row that is current until a block is selected", async () => {
    const get = renderHarness(nestedDoc)
    await waitFor(() => expect(get()).toBeTruthy())
    const root = screen.getByTestId("ve-outline-root")
    expect(root.className).toContain("is-selected")
    const headingPos = topLevelPositions(get()!)[0]
    fireEvent.click(screen.getByTestId(`ve-outline-select-${headingPos}`))
    await waitFor(() => expect(screen.getByTestId("ve-outline-root").className).not.toContain("is-selected"))
    fireEvent.click(screen.getByText("Email"))
    await waitFor(() => expect(screen.getByTestId("ve-outline-root").className).toContain("is-selected"))
  })

  it("hides row menus and dragging when read-only", async () => {
    const get = renderHarness(threeParas, false)
    await waitFor(() => expect(get()).toBeTruthy())
    expect(screen.queryByTestId(`ve-outline-menu-${topLevelPositions(get()!)[0]}`)).toBeNull()
    expect(screen.getAllByTestId("ve-outline-row-paragraph")[0].getAttribute("draggable")).toBe("false")
  })
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels/StructureOutline.test.tsx`
Expected: FAIL. No `ve-outline-root` and no `ve-outline-menu-*`.

- [ ] **Step 3: Implement**

In `StructureOutline.tsx`:

1. Imports, added:
```ts
import { EllipsisVertical } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
```
2. `type Props = { editor: Editor; selection: BlockSelection; editable?: boolean }`.
3. Replace the `StructureOutline` component's returned JSX with:
```tsx
  return (
    <div className="ve-panel ve-panel--structure" data-testid="ve-structure-panel">
      <div className="ve-panel__title">{t("panel.structureTitle")}</div>
      <ul className="ve-outline" role="tree" data-testid="ve-outline">
        <li className="ve-outline__item" role="treeitem" aria-selected={selection.selectedPos === null}>
          <div
            className={`ve-outline__row ve-outline__row--root${selection.selectedPos === null ? " is-selected" : ""}`}
            data-testid="ve-outline-root"
          >
            <button type="button" className="ve-outline__label" onClick={() => selection.clear()}>
              <span className="ve-outline__type">{t("screen.rootLabel")}</span>
            </button>
          </div>
          {entries.length === 0 ? (
            <p className="ve-panel__empty">{t("structure.empty")}</p>
          ) : (
            <ul className="ve-outline" role="group">
              {entries.map((entry) => (
                <OutlineNode
                  key={entry.pos}
                  entry={entry}
                  depth={1}
                  editor={editor}
                  selection={selection}
                  collapsed={collapsed}
                  setCollapsed={setCollapsed}
                  editable={editable}
                  t={t}
                />
              ))}
            </ul>
          )}
        </li>
      </ul>
    </div>
  )
```
and destructure `editable = true` in the component signature.
4. Replace `labelFor`'s signature (body unchanged) and export it:
```ts
export type BlockLabelInput = { type: string; level?: number; count?: number }

export function labelFor(entry: BlockLabelInput, t: TFunction<"visualEditor">): string {
```
5. In `OutlineNode`: add `editable: boolean` to its props and pass `editable={editable}` to the recursive children. Set `draggable={editable}` on the row div and make `onDragStart`/`onDrop` return early when `!editable`. Replace the `<span className="ve-outline__actions">…</span>` block with:
```tsx
        {editable && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className="ve-outline__menu"
                aria-label={t("structure.actions.menu")}
                data-testid={`ve-outline-menu-${entry.pos}`}
              >
                <EllipsisVertical className="size-4" aria-hidden />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                data-testid={`ve-outline-duplicate-${entry.pos}`}
                onSelect={() => duplicateBlock(editor, entry.pos)}
              >
                {t("structure.actions.duplicate")}
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                data-testid={`ve-outline-delete-${entry.pos}`}
                onSelect={() => deleteBlock(editor, entry.pos)}
              >
                {t("structure.actions.delete")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
```
6. Add `export` to `function deleteBlock` and `function duplicateBlock`.
7. Update the header comment: the outline lives in the right panel under a root «Email» row; row actions sit in a ⋮ menu; read-only hides menus and dragging.

- [ ] **Step 4: CSS**

In `visual-editor.css`, delete the `.ve-outline__actions`, `.ve-outline__row:hover .ve-outline__actions` and `.ve-outline__actions button` rules, and append:
```css
.ve-outline__menu {
  display: inline-flex;
  padding: 2px;
  border: none;
  border-radius: 4px;
  background: none;
  color: #6b7280;
  cursor: pointer;
  opacity: 0;
}

.ve-outline__row:hover .ve-outline__menu,
.ve-outline__menu[aria-expanded="true"],
.ve-outline__menu:focus-visible {
  opacity: 1;
}

.ve-outline__row--root .ve-outline__type {
  font-weight: 600;
}
```

- [ ] **Step 5: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels src/components/visual-editor/ThreePaneEditor.test.tsx`
Expected: `EXIT:0`.

- [ ] **Step 6: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/panels/StructureOutline.tsx src/components/visual-editor/panels/StructureOutline.test.tsx && cd ..
git add frontend/src/components/visual-editor/panels/StructureOutline.tsx frontend/src/components/visual-editor/panels/StructureOutline.test.tsx frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): layers panel with root Email row and row action menu"
```

---

### Task 8: Block library

**Files:**
- Create: `frontend/src/components/visual-editor/panels/BlockLibrary.tsx`
- Create: `frontend/src/components/visual-editor/panels/BlockLibrary.test.tsx`
- Create: `frontend/src/components/visual-editor/panels/BlockLibrary.stories.tsx`
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Consumes: `BLOCK_CATALOG`, `CATALOG_TYPES`, `CatalogItem`, `CatalogKey`, `CatalogType` (Task 3).
- Produces: `BlockLibrary({ open: boolean; onOpenChange: (open: boolean) => void; onInsert: (item: CatalogItem) => void })`. Test ids: `ve-library`, `ve-library-type-${type}`, `ve-library-item-${key}`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/visual-editor/panels/BlockLibrary.test.tsx`:
```tsx
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { BlockLibrary } from "./BlockLibrary"

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("BlockLibrary", () => {
  it("renders nothing while closed", () => {
    render(<BlockLibrary open={false} onOpenChange={vi.fn()} onInsert={vi.fn()} />)
    expect(screen.queryByTestId("ve-library")).toBeNull()
  })

  it("lists block types and shows Text variants first", () => {
    render(<BlockLibrary open onOpenChange={vi.fn()} onInsert={vi.fn()} />)
    expect(screen.getByText("Block library")).toBeTruthy()
    expect(screen.getByTestId("ve-library-type-heading")).toBeTruthy()
    expect(screen.getByTestId("ve-library-item-paragraph")).toBeTruthy()
    expect(screen.queryByTestId("ve-library-item-heading-1")).toBeNull()
  })

  it("switches type and inserts the clicked variant", () => {
    const onInsert = vi.fn()
    render(<BlockLibrary open onOpenChange={vi.fn()} onInsert={onInsert} />)
    fireEvent.click(screen.getByTestId("ve-library-type-heading"))
    expect(screen.getByTestId("ve-library-item-heading-3")).toBeTruthy()
    fireEvent.click(screen.getByTestId("ve-library-item-heading-2"))
    expect(onInsert).toHaveBeenCalledWith(expect.objectContaining({ key: "heading-2", type: "heading" }))
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels/BlockLibrary.test.tsx`
Expected: FAIL. Module not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/visual-editor/panels/BlockLibrary.tsx`:
```tsx
// BlockLibrary — the slide-out «Block library» opened by «Add block» or the
// empty-canvas button. Left: block types under the «Basic» group (phase 2 adds
// «Sections», phase 4 «My sections»). Right: variant cards for the active type
// with tiny static previews. Clicking a card hands the catalog item to the
// caller, which inserts it (see blockCatalog.insertBlock).

import { useState } from "react"
import { useTranslation } from "react-i18next"
import { BLOCK_CATALOG, CATALOG_TYPES } from "../blockCatalog"
import type { CatalogItem, CatalogKey, CatalogType } from "../blockCatalog"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onInsert: (item: CatalogItem) => void
}

export function BlockLibrary({ open, onOpenChange, onInsert }: Props) {
  const { t } = useTranslation("visualEditor")
  const [active, setActive] = useState<CatalogType>("text")
  const items = BLOCK_CATALOG.filter((i) => i.type === active)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        className="w-[680px] max-w-[95vw] gap-0 p-0 sm:max-w-[680px]"
        data-testid="ve-library"
      >
        <SheetHeader>
          <SheetTitle>{t("library.title")}</SheetTitle>
          <SheetDescription className="sr-only">{t("library.title")}</SheetDescription>
        </SheetHeader>
        <div className="ve-library__body">
          <nav className="ve-library__nav">
            <p className="ve-library__group">{t("library.group.basic")}</p>
            {CATALOG_TYPES.map((type) => (
              <button
                key={type}
                type="button"
                className={`ve-library__type${type === active ? " is-active" : ""}`}
                aria-current={type === active}
                data-testid={`ve-library-type-${type}`}
                onClick={() => setActive(type)}
              >
                {t(`catalog.type.${type}`)}
              </button>
            ))}
          </nav>
          <div className="ve-library__cards">
            {items.map((item) => (
              <button
                key={item.key}
                type="button"
                className="ve-library__card"
                data-testid={`ve-library-item-${item.key}`}
                onClick={() => onInsert(item)}
              >
                <CardPreview itemKey={item.key} />
                <span className="ve-library__card-label">{t(`catalog.item.${item.key}`)}</span>
              </button>
            ))}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}

// CardPreview draws a tiny static mock of the block. No image assets to keep.
function CardPreview({ itemKey }: { itemKey: CatalogKey }) {
  switch (itemKey) {
    case "heading-1":
      return <span className="ve-mock ve-mock--h1">Aa</span>
    case "heading-2":
      return <span className="ve-mock ve-mock--h2">Aa</span>
    case "heading-3":
      return <span className="ve-mock ve-mock--h3">Aa</span>
    case "columns-2":
    case "columns-3":
    case "columns-4":
      return (
        <span className="ve-mock ve-mock--columns">
          {Array.from({ length: Number(itemKey.slice(-1)) }, (_, i) => (
            <span key={i} />
          ))}
        </span>
      )
    case "button":
      return <span className="ve-mock ve-mock--button" />
    case "image":
      return <span className="ve-mock ve-mock--image" />
    case "divider":
      return <span className="ve-mock ve-mock--divider" />
    case "raw-html":
      return <span className="ve-mock ve-mock--code">{"</>"}</span>
    case "code":
      return <span className="ve-mock ve-mock--code">{"{ }"}</span>
    case "bullet-list":
    case "ordered-list":
      return (
        <span className="ve-mock ve-mock--list">
          <span />
          <span />
          <span />
        </span>
      )
    case "quote":
      return (
        <span className="ve-mock ve-mock--quote">
          <span />
          <span />
        </span>
      )
    case "paragraph":
      return (
        <span className="ve-mock ve-mock--lines">
          <span />
          <span />
          <span />
        </span>
      )
  }
}
```

- [ ] **Step 4: CSS**

Append to `visual-editor.css`:
```css
/* ── Block library (editor shell 2026-11-14) ───────────────────────────── */

.ve-library__body {
  display: grid;
  grid-template-columns: 200px 1fr;
  flex: 1 1 auto;
  min-height: 0;
  border-top: 1px solid #e5e7eb;
}

.ve-library__nav {
  display: flex;
  flex-direction: column;
  padding: 12px 0;
  overflow: auto;
  border-right: 1px solid #e5e7eb;
}

.ve-library__group {
  margin: 0;
  padding: 4px 16px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: #9ca3af;
}

.ve-library__type {
  padding: 8px 24px;
  border: none;
  background: none;
  text-align: left;
  font-size: 14px;
  cursor: pointer;
}

.ve-library__type:hover {
  background: #f4f4f5;
}

.ve-library__type.is-active {
  background: #ede9fe;
}

.ve-library__cards {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  overflow: auto;
  background: #f4f4f5;
}

.ve-library__card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px 16px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
}

.ve-library__card:hover {
  border-color: #2563eb;
}

.ve-library__card-label {
  font-size: 12px;
  color: #6b7280;
}

.ve-mock {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 70%;
  min-height: 48px;
  color: #111827;
}

.ve-mock--h1 { font-size: 32px; font-weight: 700; }
.ve-mock--h2 { font-size: 24px; font-weight: 700; }
.ve-mock--h3 { font-size: 18px; font-weight: 700; }

.ve-mock--lines,
.ve-mock--list,
.ve-mock--quote {
  flex-direction: column;
  align-items: stretch;
}

.ve-mock--lines > span,
.ve-mock--list > span,
.ve-mock--quote > span {
  height: 6px;
  border-radius: 3px;
  background: #d4d4d8;
}

.ve-mock--list > span { margin-left: 14px; }
.ve-mock--quote { border-left: 3px solid #a1a1aa; padding-left: 10px; }

.ve-mock--columns > span {
  flex: 1;
  height: 48px;
  border: 1px dashed #a1a1aa;
  border-radius: 4px;
}

.ve-mock--button { flex: none; width: 96px; min-height: 28px; height: 28px; border-radius: 6px; background: #2563eb; }
.ve-mock--image { height: 56px; border-radius: 6px; background: linear-gradient(135deg, #e4e4e7, #d4d4d8); }
.ve-mock--divider { min-height: 1px; height: 1px; background: #a1a1aa; }
.ve-mock--code { font-family: ui-monospace, monospace; font-size: 20px; color: #6b7280; }
```

- [ ] **Step 5: Story**

Create `frontend/src/components/visual-editor/panels/BlockLibrary.stories.tsx`:
```tsx
import { expect, fn, userEvent, within } from "storybook/test"
import { BlockLibrary } from "./BlockLibrary"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: BlockLibrary,
  args: { open: true, onOpenChange: fn(), onInsert: fn() },
} satisfies Meta<typeof BlockLibrary>

export default meta
type Story = StoryObj<typeof meta>

export const Open: Story = {}

export const PickHeading: Story = {
  play: async ({ args }) => {
    // The sheet renders in a portal on document.body.
    const body = within(document.body)
    await userEvent.click(await body.findByTestId("ve-library-type-heading"))
    await userEvent.click(body.getByTestId("ve-library-item-heading-1"))
    await expect(args.onInsert).toHaveBeenCalled()
  },
}
```

- [ ] **Step 6: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/panels/BlockLibrary`
Expected: `EXIT:0` (unit + story).

- [ ] **Step 7: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/panels/BlockLibrary.tsx src/components/visual-editor/panels/BlockLibrary.test.tsx src/components/visual-editor/panels/BlockLibrary.stories.tsx && cd ..
git add frontend/src/components/visual-editor/panels/BlockLibrary.tsx frontend/src/components/visual-editor/panels/BlockLibrary.test.tsx frontend/src/components/visual-editor/panels/BlockLibrary.stories.tsx frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): slide-out block library"
```

---

### Task 9: Floating block toolbar

**Files:**
- Create: `frontend/src/components/visual-editor/ui/BlockToolbar.tsx`
- Create: `frontend/src/components/visual-editor/ui/BlockToolbar.test.tsx`
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Consumes: `labelFor`, `deleteBlock`, `duplicateBlock` (Task 7); `BlockSelection`.
- Produces: `BlockToolbar({ editor: Editor; selection: BlockSelection; pageRef: RefObject<HTMLDivElement | null> })`. Test ids: `ve-block-toolbar`, `ve-block-toolbar-duplicate`, `ve-block-toolbar-delete`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/visual-editor/ui/BlockToolbar.test.tsx`:
```tsx
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { useEffect, useRef } from "react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Divider } from "../extensions/Divider"
import { useBlockSelection } from "../hooks/useBlockSelection"
import { BlockToolbar } from "./BlockToolbar"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Editor } from "@tiptap/core"

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const content = {
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }, { type: "divider" }],
}

function Harness({ onReady }: { onReady: (e: Editor, s: BlockSelection) => void }) {
  const editor = useEditor({ extensions: [StarterKit.configure({ hardBreak: false }), Divider], content })
  const selection = useBlockSelection(editor)
  const pageRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => onReady(editor, selection))
  return (
    <div ref={pageRef} style={{ position: "relative" }}>
      <EditorContent editor={editor} />
      <BlockToolbar editor={editor} selection={selection} pageRef={pageRef} />
    </div>
  )
}

function setup() {
  let editor: Editor | null = null
  let selection: BlockSelection | null = null
  render(<Harness onReady={(e, s) => ((editor = e), (selection = s))} />)
  return { editor: () => editor!, selection: () => selection! }
}

function dividerPos(editor: Editor): number {
  let pos = -1
  editor.state.doc.forEach((n, p) => {
    if (n.type.name === "divider") pos = p
  })
  return pos
}

describe("BlockToolbar", () => {
  it("is hidden until a block is selected", async () => {
    const h = setup()
    await waitFor(() => expect(h.editor()).toBeTruthy())
    expect(screen.queryByTestId("ve-block-toolbar")).toBeNull()
  })

  it("labels the selected block and duplicates / deletes it", async () => {
    const h = setup()
    await waitFor(() => expect(h.editor()).toBeTruthy())
    h.selection().selectBlock(dividerPos(h.editor()))
    expect(await screen.findByText("Divider")).toBeTruthy()
    fireEvent.click(screen.getByTestId("ve-block-toolbar-duplicate"))
    await waitFor(() => {
      let dividers = 0
      h.editor().state.doc.forEach((n) => (dividers += n.type.name === "divider" ? 1 : 0))
      expect(dividers).toBe(2)
    })
    fireEvent.click(screen.getByTestId("ve-block-toolbar-delete"))
    await waitFor(() => {
      let dividers = 0
      h.editor().state.doc.forEach((n) => (dividers += n.type.name === "divider" ? 1 : 0))
      expect(dividers).toBe(1)
    })
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/ui/BlockToolbar.test.tsx`
Expected: FAIL. Module not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/visual-editor/ui/BlockToolbar.tsx`:
```tsx
// BlockToolbar — the floating label centered under the selected block: the
// block's name (same label as in Layers) plus Duplicate / Delete. It is
// absolutely positioned inside the white page from the block's DOM rect, so it
// scrolls with the canvas and never competes with the text bubble menu.

import { useLayoutEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Copy, Trash2 } from "lucide-react"
import { deleteBlock, duplicateBlock, labelFor } from "../panels/StructureOutline"
import type { BlockSelection } from "../hooks/useBlockSelection"
import type { Editor } from "@tiptap/core"
import type { RefObject } from "react"

type Props = {
  editor: Editor
  selection: BlockSelection
  pageRef: RefObject<HTMLDivElement | null>
}

export function BlockToolbar({ editor, selection, pageRef }: Props) {
  const { t } = useTranslation("visualEditor")
  const { selectedPos, selectedNode } = selection
  const [coords, setCoords] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    const page = pageRef.current
    const dom = selectedPos == null ? null : editor.view.nodeDOM(selectedPos)
    if (!page || !(dom instanceof HTMLElement)) {
      setCoords(null)
      return
    }
    const block = dom.getBoundingClientRect()
    const box = page.getBoundingClientRect()
    setCoords({ top: block.bottom - box.top + 8, left: block.left - box.left + block.width / 2 })
  }, [editor, selectedPos, selectedNode, pageRef])

  if (selectedPos == null || selectedNode == null || coords == null) return null

  const label = labelFor(
    {
      type: selectedNode.type.name,
      level: selectedNode.attrs.level as number | undefined,
      count: selectedNode.attrs.count as number | undefined,
    },
    t,
  )

  return (
    <div className="ve-block-toolbar" data-testid="ve-block-toolbar" style={{ top: coords.top, left: coords.left }}>
      <span className="ve-block-toolbar__label">{label}</span>
      <button
        type="button"
        aria-label={t("structure.actions.duplicate")}
        title={t("structure.actions.duplicate")}
        data-testid="ve-block-toolbar-duplicate"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => duplicateBlock(editor, selectedPos)}
      >
        <Copy className="size-4" aria-hidden />
      </button>
      <button
        type="button"
        aria-label={t("structure.actions.delete")}
        title={t("structure.actions.delete")}
        data-testid="ve-block-toolbar-delete"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => deleteBlock(editor, selectedPos)}
      >
        <Trash2 className="size-4" aria-hidden />
      </button>
    </div>
  )
}
```

- [ ] **Step 4: CSS**

Append to `visual-editor.css`:
```css
.ve-block-toolbar {
  position: absolute;
  z-index: 20;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 6px;
  background: #2563eb;
  color: #fff;
  font-size: 12px;
  box-shadow: 0 2px 6px rgba(37, 99, 235, 0.3);
}

.ve-block-toolbar__label {
  margin-right: 4px;
  font-weight: 500;
}

.ve-block-toolbar button {
  display: inline-flex;
  padding: 2px;
  border: none;
  border-radius: 4px;
  background: none;
  color: inherit;
  cursor: pointer;
}

.ve-block-toolbar button:hover {
  background: rgba(255, 255, 255, 0.2);
}
```

- [ ] **Step 5: Run the test**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/ui/BlockToolbar.test.tsx`
Expected: `EXIT:0`.

- [ ] **Step 6: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/ui/BlockToolbar.tsx src/components/visual-editor/ui/BlockToolbar.test.tsx && cd ..
git add frontend/src/components/visual-editor/ui/BlockToolbar.tsx frontend/src/components/visual-editor/ui/BlockToolbar.test.tsx frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): floating toolbar under the selected block"
```

---

### Task 10: `useVisualSave` — session save state and the 409 flow

**Files:**
- Create: `frontend/src/components/visual-editor/hooks/useVisualSave.ts`
- Create: `frontend/src/components/visual-editor/hooks/useVisualSave.test.tsx`

**Interfaces:**
- Produces:
  - `type VisualSnapshot = { bodyDoc: VisualDoc; theme: Theme | null }`
  - `type VisualSaveFn = (input: VisualSnapshot & { ifUnmodifiedSince: string }) => Promise<{ data: { updatedAt: string; warnings: Array<RenderWarning> } }>`
  - `type VisualRefetchFn = () => Promise<VisualSnapshot & { updatedAt: string }>`
  - `useVisualSave({ initial, initiallyDirty?, save, refetch, onSaved? })` → `{ bodyDoc, setBodyDoc, theme, setTheme, dirty, saving, save }`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/visual-editor/hooks/useVisualSave.test.tsx`:
```tsx
import { afterEach, describe, expect, it, vi } from "vitest"
import { act, renderHook, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { toast } from "sonner"
import { useVisualSave } from "./useVisualSave"
import type { ReactNode } from "react"
import type { VisualDoc } from "@/lib/api-types"
import { ApiError } from "@/lib/errors"

vi.mock("sonner", () => ({ toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() } }))

afterEach(() => vi.clearAllMocks())

const doc = (text: string): VisualDoc => ({
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text }] }],
})

const initial = { bodyDoc: doc("a"), theme: null, updatedAt: "2026-01-01T00:00:00Z" }

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

const okSave = (updatedAt: string) =>
  vi.fn().mockResolvedValue({ data: { updatedAt, warnings: [] } })

const stale = () =>
  new ApiError(409, "stale_row", "stale", "/x", { kind: "stale_row", currentUpdatedAt: "2026-03-03T00:00:00Z" })

describe("useVisualSave", () => {
  it("tracks dirty state and saves the current snapshot with the row token", async () => {
    const save = okSave("2026-02-02T00:00:00Z")
    const onSaved = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn(), onSaved }), { wrapper })
    expect(result.current.dirty).toBe(false)
    act(() => result.current.setBodyDoc(doc("b")))
    expect(result.current.dirty).toBe(true)
    act(() => result.current.save())
    await waitFor(() => expect(result.current.dirty).toBe(false))
    expect(save).toHaveBeenCalledWith({ bodyDoc: doc("b"), theme: null, ifUnmodifiedSince: initial.updatedAt })
    expect(onSaved).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()
  })

  it("uses the token from the previous save on the next one", async () => {
    const save = okSave("2026-02-02T00:00:00Z")
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn() }), { wrapper })
    act(() => result.current.setBodyDoc(doc("b")))
    act(() => result.current.save())
    await waitFor(() => expect(result.current.dirty).toBe(false))
    act(() => result.current.setBodyDoc(doc("c")))
    act(() => result.current.save())
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save.mock.calls[1][0].ifUnmodifiedSince).toBe("2026-02-02T00:00:00Z")
  })

  it("starts dirty when asked (freshly converted document)", () => {
    const { result } = renderHook(
      () => useVisualSave({ initial, initiallyDirty: true, save: vi.fn(), refetch: vi.fn() }),
      { wrapper },
    )
    expect(result.current.dirty).toBe(true)
  })

  it("on 409 offers Force overwrite, which re-sends the operator's doc with the fresh token", async () => {
    const save = vi
      .fn()
      .mockRejectedValueOnce(stale())
      .mockResolvedValueOnce({ data: { updatedAt: "2026-03-04T00:00:00Z", warnings: [] } })
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn() }), { wrapper })
    act(() => result.current.setBodyDoc(doc("mine")))
    act(() => result.current.save())
    await waitFor(() => expect(toast.warning).toHaveBeenCalled())
    const opts = vi.mocked(toast.warning).mock.calls[0][1] as { cancel: { onClick: () => void } }
    act(() => opts.cancel.onClick())
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save.mock.calls[1][0]).toEqual({
      bodyDoc: doc("mine"),
      theme: null,
      ifUnmodifiedSince: "2026-03-03T00:00:00Z",
    })
  })

  it("on 409 offers Reload, which replaces the doc with the server copy", async () => {
    const save = vi.fn().mockRejectedValueOnce(stale())
    const refetch = vi.fn().mockResolvedValue({ bodyDoc: doc("server"), theme: null, updatedAt: "2026-03-03T00:00:00Z" })
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch }), { wrapper })
    act(() => result.current.setBodyDoc(doc("mine")))
    act(() => result.current.save())
    await waitFor(() => expect(toast.warning).toHaveBeenCalled())
    const opts = vi.mocked(toast.warning).mock.calls[0][1] as { action: { onClick: () => void } }
    act(() => opts.action.onClick())
    await waitFor(() => expect(result.current.bodyDoc).toEqual(doc("server")))
    expect(result.current.dirty).toBe(false)
  })

  it("toasts other errors", async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error("boom"))
    const { result } = renderHook(() => useVisualSave({ initial, save, refetch: vi.fn() }), { wrapper })
    act(() => result.current.save())
    await waitFor(() => expect(toast.error).toHaveBeenCalled())
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/hooks/useVisualSave.test.tsx`
Expected: FAIL. Module not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/visual-editor/hooks/useVisualSave.ts`:
```ts
// useVisualSave — the editing session's save state, shared by the template and
// campaign editors: the working document + theme, a dirty flag (current vs
// last-saved snapshot), and the visual save with the FR-009 optimistic-
// concurrency flow. On 409 stale_row a toast offers Reload (take the server
// copy) or Force overwrite (re-send the operator's copy with a fresh token).

import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { useMutation } from "@tanstack/react-query"
import { toast } from "sonner"
import type { RenderWarning, Theme, VisualDoc } from "@/lib/api-types"
import { ApiError, errorMessage } from "@/lib/errors"

export type VisualSnapshot = { bodyDoc: VisualDoc; theme: Theme | null }

export type VisualSaveFn = (
  input: VisualSnapshot & { ifUnmodifiedSince: string },
) => Promise<{ data: { updatedAt: string; warnings: Array<RenderWarning> } }>

export type VisualRefetchFn = () => Promise<VisualSnapshot & { updatedAt: string }>

type Options = {
  initial: VisualSnapshot & { updatedAt: string }
  // True when the starting document is not what the server holds (a freshly
  // converted legacy row), so the session starts dirty.
  initiallyDirty?: boolean
  save: VisualSaveFn
  refetch: VisualRefetchFn
  onSaved?: () => Promise<void>
}

const keyOf = (s: VisualSnapshot): string => JSON.stringify([s.bodyDoc, s.theme])

export function useVisualSave({ initial, initiallyDirty = false, save, refetch, onSaved }: Options) {
  const { t } = useTranslation("visualEditor")
  const [bodyDoc, setBodyDoc] = useState(initial.bodyDoc)
  const [theme, setTheme] = useState(initial.theme)
  const [savedKey, setSavedKey] = useState<string | null>(() => (initiallyDirty ? null : keyOf(initial)))
  // The row's last-known updated_at (FR-009 token). A ref, not state, so a
  // retry issued from a toast callback always reads the latest value.
  const token = useRef(initial.updatedAt)
  const dirty = savedKey !== keyOf({ bodyDoc, theme })

  async function reload() {
    const fresh = await refetch()
    setBodyDoc(fresh.bodyDoc)
    setTheme(fresh.theme)
    token.current = fresh.updatedAt
    setSavedKey(keyOf(fresh))
  }

  const mutation = useMutation({
    mutationFn: (v: { snapshot: VisualSnapshot; ifUnmodifiedSince: string }) =>
      save({ ...v.snapshot, ifUnmodifiedSince: v.ifUnmodifiedSince }),
    onSuccess: async (res, v) => {
      token.current = res.data.updatedAt
      // Mark what was *sent* as saved: edits typed during the request stay dirty.
      setSavedKey(keyOf(v.snapshot))
      await onSaved?.()
      const warnings = res.data.warnings.length
      if (warnings > 0) toast.warning(t("save.warnings", { count: warnings }))
      else toast.success(t("save.success"))
    },
    onError: (e, v) => {
      if (e instanceof ApiError && e.status === 409 && e.slug === "stale_row") {
        const current = typeof e.data?.currentUpdatedAt === "string" ? e.data.currentUpdatedAt : null
        toast.warning(t("save.staleTitle"), {
          duration: 12_000,
          action: {
            label: t("save.reload"),
            onClick: () => {
              reload().catch((err: unknown) => toast.error(errorMessage(err)))
            },
          },
          cancel: {
            label: t("save.forceOverwrite"),
            onClick: () => {
              void forceOverwrite(v.snapshot, current)
            },
          },
        })
        return
      }
      toast.error(errorMessage(e))
    },
  })

  async function forceOverwrite(snapshot: VisualSnapshot, currentUpdatedAt: string | null) {
    try {
      const ifUnmodifiedSince = currentUpdatedAt ?? (await refetch()).updatedAt
      mutation.mutate({ snapshot, ifUnmodifiedSince })
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return {
    bodyDoc,
    setBodyDoc,
    theme,
    setTheme,
    dirty,
    saving: mutation.isPending,
    save: () => mutation.mutate({ snapshot: { bodyDoc, theme }, ifUnmodifiedSince: token.current }),
  }
}
```

- [ ] **Step 4: Run the test**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/hooks/useVisualSave.test.tsx`
Expected: `EXIT:0`.

- [ ] **Step 5: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/hooks/useVisualSave.ts src/components/visual-editor/hooks/useVisualSave.test.tsx && cd ..
git add frontend/src/components/visual-editor/hooks/useVisualSave.ts frontend/src/components/visual-editor/hooks/useVisualSave.test.tsx
git commit -m "feat(visual-editor): shared visual save session with stale-row handling"
```

---

### Task 11: Editor top bar

**Files:**
- Create: `frontend/src/components/visual-editor/ui/EditorTopBar.tsx`
- Create: `frontend/src/components/visual-editor/ui/EditorTopBar.test.tsx`
- Create: `frontend/src/components/visual-editor/ui/EditorTopBar.stories.tsx`
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Produces: `type Device = "desktop" | "mobile"`, and `EditorTopBar(props: EditorTopBarProps)` with
  `{ section: string; name: string; onBack: () => void; editable: boolean; device: Device; onDeviceChange: (d: Device) => void; canUndo: boolean; canRedo: boolean; onUndo: () => void; onRedo: () => void; onAdd: () => void; onPreview: () => void; dirty: boolean; saving: boolean; onSave: () => void; onOpenPanel?: () => void }`.
  Test ids: `ve-topbar`, `ve-back`, `ve-topbar-name`, `ve-add`, `ve-undo`, `ve-redo`, `ve-device-desktop`, `ve-device-mobile`, `ve-preview`, `ve-save`, `ve-readonly-badge`, `ve-open-panel`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/visual-editor/ui/EditorTopBar.test.tsx`:
```tsx
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { EditorTopBar } from "./EditorTopBar"
import type { ComponentProps } from "react"

afterEach(() => cleanup())

function props(over: Partial<ComponentProps<typeof EditorTopBar>> = {}): ComponentProps<typeof EditorTopBar> {
  return {
    section: "Templates",
    name: "Welcome",
    onBack: vi.fn(),
    editable: true,
    device: "desktop",
    onDeviceChange: vi.fn(),
    canUndo: false,
    canRedo: false,
    onUndo: vi.fn(),
    onRedo: vi.fn(),
    onAdd: vi.fn(),
    onPreview: vi.fn(),
    dirty: false,
    saving: false,
    onSave: vi.fn(),
    ...over,
  }
}

describe("EditorTopBar", () => {
  it("shows breadcrumbs and wires back / add / preview", () => {
    const p = props()
    render(<EditorTopBar {...p} />)
    expect(screen.getByTestId("ve-topbar-name").textContent).toBe("Welcome")
    expect(screen.getByText("Templates")).toBeTruthy()
    fireEvent.click(screen.getByTestId("ve-back"))
    fireEvent.click(screen.getByTestId("ve-add"))
    fireEvent.click(screen.getByTestId("ve-preview"))
    expect(p.onBack).toHaveBeenCalled()
    expect(p.onAdd).toHaveBeenCalled()
    expect(p.onPreview).toHaveBeenCalled()
  })

  it("disables undo/redo per editor history", () => {
    render(<EditorTopBar {...props({ canUndo: true })} />)
    expect((screen.getByTestId("ve-undo") as HTMLButtonElement).disabled).toBe(false)
    expect((screen.getByTestId("ve-redo") as HTMLButtonElement).disabled).toBe(true)
  })

  it("marks the current device and switches it", () => {
    const p = props()
    render(<EditorTopBar {...p} />)
    expect(screen.getByTestId("ve-device-desktop").getAttribute("aria-pressed")).toBe("true")
    fireEvent.click(screen.getByTestId("ve-device-mobile"))
    expect(p.onDeviceChange).toHaveBeenCalledWith("mobile")
  })

  it("enables Save only when dirty and labels its state", () => {
    const { rerender } = render(<EditorTopBar {...props()} />)
    const save = () => screen.getByTestId("ve-save") as HTMLButtonElement
    expect(save().disabled).toBe(true)
    expect(save().textContent).toBe("Saved")
    rerender(<EditorTopBar {...props({ dirty: true })} />)
    expect(save().disabled).toBe(false)
    expect(save().textContent).toBe("Save")
    rerender(<EditorTopBar {...props({ dirty: true, saving: true })} />)
    expect(save().disabled).toBe(true)
    expect(save().textContent).toBe("Saving…")
  })

  it("read-only hides editing controls and shows a badge", () => {
    render(<EditorTopBar {...props({ editable: false })} />)
    expect(screen.getByTestId("ve-readonly-badge")).toBeTruthy()
    expect(screen.queryByTestId("ve-add")).toBeNull()
    expect(screen.queryByTestId("ve-undo")).toBeNull()
    expect(screen.queryByTestId("ve-save")).toBeNull()
  })

  it("shows the panel button only when given (narrow viewport)", () => {
    const { rerender } = render(<EditorTopBar {...props()} />)
    expect(screen.queryByTestId("ve-open-panel")).toBeNull()
    rerender(<EditorTopBar {...props({ onOpenPanel: vi.fn() })} />)
    expect(screen.getByTestId("ve-open-panel")).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/ui/EditorTopBar.test.tsx`
Expected: FAIL. Module not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/visual-editor/ui/EditorTopBar.tsx`:
```tsx
// EditorTopBar — the full-screen editor's top bar: back + breadcrumbs, «Add
// block», undo/redo, desktop/mobile canvas width, Preview, and Save. Read-only
// sessions show a badge instead of the editing controls.

import { useTranslation } from "react-i18next"
import { ArrowLeft, Eye, Monitor, PanelRight, Plus, Redo2, Smartphone, Undo2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

export type Device = "desktop" | "mobile"

type Props = {
  section: string
  name: string
  onBack: () => void
  editable: boolean
  device: Device
  onDeviceChange: (device: Device) => void
  canUndo: boolean
  canRedo: boolean
  onUndo: () => void
  onRedo: () => void
  onAdd: () => void
  onPreview: () => void
  dirty: boolean
  saving: boolean
  onSave: () => void
  // Present only on narrow viewports, where the right panel is a sheet.
  onOpenPanel?: () => void
}

const DEVICES: ReadonlyArray<Device> = ["desktop", "mobile"]

export function EditorTopBar(p: Props) {
  const { t } = useTranslation("visualEditor")
  const saveLabel = p.saving ? t("screen.saving") : p.dirty ? t("screen.save") : t("screen.saved")
  return (
    <header className="ve-topbar" data-testid="ve-topbar">
      <div className="ve-topbar__group">
        <Button type="button" variant="ghost" size="icon-sm" onClick={p.onBack} aria-label={t("screen.back")} data-testid="ve-back">
          <ArrowLeft />
        </Button>
        <nav className="ve-topbar__crumbs" aria-label={t("screen.back")}>
          <span>{p.section}</span>
          <span aria-hidden>/</span>
          <span className="ve-topbar__name" data-testid="ve-topbar-name">
            {p.name}
          </span>
        </nav>
        {p.editable ? (
          <>
            <Button type="button" size="sm" onClick={p.onAdd} data-testid="ve-add">
              <Plus /> {t("screen.add")}
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" onClick={p.onUndo} disabled={!p.canUndo} aria-label={t("screen.undo")} data-testid="ve-undo">
              <Undo2 />
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" onClick={p.onRedo} disabled={!p.canRedo} aria-label={t("screen.redo")} data-testid="ve-redo">
              <Redo2 />
            </Button>
          </>
        ) : (
          <Badge variant="secondary" data-testid="ve-readonly-badge">
            {t("screen.readOnly")}
          </Badge>
        )}
      </div>
      <div className="ve-topbar__group" role="group" aria-label={t("screen.device.label")}>
        {DEVICES.map((d) => (
          <Button
            key={d}
            type="button"
            variant={p.device === d ? "secondary" : "ghost"}
            size="icon-sm"
            aria-pressed={p.device === d}
            aria-label={t(`screen.device.${d}`)}
            data-testid={`ve-device-${d}`}
            onClick={() => p.onDeviceChange(d)}
          >
            {d === "desktop" ? <Monitor /> : <Smartphone />}
          </Button>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={p.onPreview} data-testid="ve-preview">
          <Eye /> {t("screen.preview")}
        </Button>
      </div>
      <div className="ve-topbar__group">
        {p.onOpenPanel && (
          <Button type="button" variant="outline" size="sm" onClick={p.onOpenPanel} data-testid="ve-open-panel">
            <PanelRight /> {t("screen.openPanel")}
          </Button>
        )}
        {p.editable && (
          <Button type="button" size="sm" onClick={p.onSave} disabled={!p.dirty || p.saving} data-testid="ve-save">
            {saveLabel}
          </Button>
        )}
      </div>
    </header>
  )
}
```

The Save test asserts `textContent` equals the label exactly, so the Save button must render nothing but `{saveLabel}` (no icon).

- [ ] **Step 4: CSS**

Append to `visual-editor.css`:
```css
/* ── Editor screen (editor shell 2026-11-14) ───────────────────────────── */

.ve-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid #e5e7eb;
  background: #fff;
}

.ve-topbar__group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ve-topbar__crumbs {
  display: flex;
  gap: 6px;
  margin-right: 8px;
  font-size: 13px;
  color: #6b7280;
}

.ve-topbar__name {
  color: #111827;
  font-weight: 500;
}
```

- [ ] **Step 5: Story**

Create `frontend/src/components/visual-editor/ui/EditorTopBar.stories.tsx`:
```tsx
import { fn } from "storybook/test"
import { EditorTopBar } from "./EditorTopBar"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: EditorTopBar,
  args: {
    section: "Templates",
    name: "Welcome email",
    onBack: fn(),
    editable: true,
    device: "desktop",
    onDeviceChange: fn(),
    canUndo: true,
    canRedo: false,
    onUndo: fn(),
    onRedo: fn(),
    onAdd: fn(),
    onPreview: fn(),
    dirty: true,
    saving: false,
    onSave: fn(),
  },
} satisfies Meta<typeof EditorTopBar>

export default meta
type Story = StoryObj<typeof meta>

export const Editing: Story = {}
export const Saved: Story = { args: { dirty: false } }
export const ReadOnly: Story = { args: { editable: false } }
```

- [ ] **Step 6: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/ui/EditorTopBar`
Expected: `EXIT:0`.

- [ ] **Step 7: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/ui/EditorTopBar.tsx src/components/visual-editor/ui/EditorTopBar.test.tsx src/components/visual-editor/ui/EditorTopBar.stories.tsx && cd ..
git add frontend/src/components/visual-editor/ui/EditorTopBar.tsx frontend/src/components/visual-editor/ui/EditorTopBar.test.tsx frontend/src/components/visual-editor/ui/EditorTopBar.stories.tsx frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): editor top bar"
```

---

### Task 12: `EditorScreen` — conversion, session, and layout

**Files:**
- Create: `frontend/src/components/visual-editor/EditorScreen.tsx`
- Create: `frontend/src/components/visual-editor/EditorScreen.test.tsx`
- Create: `frontend/src/components/visual-editor/EditorScreen.stories.tsx`
- Modify: `frontend/src/components/visual-editor/visual-editor.css`

**Interfaces:**
- Consumes: everything from Tasks 2–11; `VisualEmailEditor` (`onEditorReady`, `theme`, `editable`); `useEditorTheme` from `./plugins/theming`; `ThemeControls`; `PreviewIframe`.
- Produces:
  - `type EditorRow = { id: string; name: string; body_doc?: VisualDoc | null; body_html: string; theme?: Theme | null; updated_at: string }` (`TemplateView` and `CampaignView` both satisfy it)
  - `type EditorScreenProps = { slug: string; section: string; row: EditorRow; editable: boolean; onBack: () => void; convert: () => Promise<{ data: ConvertToVisualResponse }>; save: VisualSaveFn; refetch: () => Promise<EditorRow>; onSaved: () => Promise<void> }`
  - `EditorScreen(props: EditorScreenProps)`
  - Test ids: `ve-screen`, `ve-canvas`, `ve-page`, `ve-empty`, `ve-empty-add`, `ve-converting`, `ve-convert-failed`, `ve-preview-dialog`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/visual-editor/EditorScreen.test.tsx`:
```tsx
// EditorScreen with the real TipTap canvas (jsdom can drive it for these
// flows). The router is mocked: only useBlocker is used.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { useBlocker } from "@tanstack/react-router"
import { EditorScreen } from "./EditorScreen"
import type { EditorRow, EditorScreenProps } from "./EditorScreen"
import type { VisualDoc } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"

vi.mock("@tanstack/react-router", () => ({ useBlocker: vi.fn() }))

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
  }
})

function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  mockMatchMedia(false)
})

const doc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }],
}

const row: EditorRow = {
  id: "tpl-1",
  name: "Welcome",
  body_doc: doc,
  body_html: "<p>hi</p>",
  theme: null,
  updated_at: "2026-01-01T00:00:00Z",
}

function renderScreen(over: Partial<EditorScreenProps> = {}): EditorScreenProps {
  const props: EditorScreenProps = {
    slug: "acme",
    section: "Templates",
    row,
    editable: true,
    onBack: vi.fn(),
    convert: vi.fn(),
    save: vi.fn().mockResolvedValue({ data: { updatedAt: "2026-01-02T00:00:00Z", warnings: [] } }),
    refetch: vi.fn(),
    onSaved: vi.fn().mockResolvedValue(undefined),
    ...over,
  }
  renderWithClient(<EditorScreen {...props} />)
  return props
}

async function insertDivider() {
  fireEvent.click(screen.getByTestId("ve-add"))
  fireEvent.click(await screen.findByTestId("ve-library-type-divider"))
  fireEvent.click(screen.getByTestId("ve-library-item-divider"))
  await waitFor(() => expect((screen.getByTestId("ve-save") as HTMLButtonElement).disabled).toBe(false))
}

describe("EditorScreen", () => {
  it("renders the top bar, canvas, layers and settings", async () => {
    renderScreen()
    expect(screen.getByTestId("ve-topbar-name").textContent).toBe("Welcome")
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await waitFor(() => expect(screen.getByTestId("ve-structure-panel")).toBeTruthy())
    expect(screen.getByTestId("ve-params-panel")).toBeTruthy()
    expect((screen.getByTestId("ve-save") as HTMLButtonElement).disabled).toBe(true)
  })

  it("inserts a block from the library and saves the new doc", async () => {
    const props = renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await insertDivider()
    fireEvent.click(screen.getByTestId("ve-save"))
    await waitFor(() => expect(props.save).toHaveBeenCalled())
    const sent = vi.mocked(props.save).mock.calls[0][0]
    expect(sent.ifUnmodifiedSince).toBe(row.updated_at)
    expect(sent.bodyDoc.content.some((b) => b.type === "divider")).toBe(true)
  })

  it("saves on Cmd/Ctrl+S when dirty", async () => {
    const props = renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    await insertDivider()
    fireEvent.keyDown(window, { key: "s", metaKey: true })
    await waitFor(() => expect(props.save).toHaveBeenCalled())
  })

  it("registers an unsaved-changes guard that follows the dirty flag", async () => {
    renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    const lastOpts = () =>
      vi.mocked(useBlocker).mock.lastCall![0] as unknown as { enableBeforeUnload: () => boolean }
    expect(lastOpts().enableBeforeUnload()).toBe(false)
    await insertDivider()
    expect(lastOpts().enableBeforeUnload()).toBe(true)
  })

  it("narrows the page on mobile", async () => {
    renderScreen()
    fireEvent.click(screen.getByTestId("ve-device-mobile"))
    expect(screen.getByTestId("ve-page").style.width).toBe("375px")
  })

  it("read-only hides editing controls", async () => {
    renderScreen({ editable: false })
    expect(screen.getByTestId("ve-readonly-badge")).toBeTruthy()
    expect(screen.queryByTestId("ve-add")).toBeNull()
    expect(screen.queryByTestId("ve-save")).toBeNull()
  })

  it("converts a legacy raw-HTML row on load and starts dirty", async () => {
    const convert = vi.fn().mockResolvedValue({ data: { bodyDoc: doc, warnings: [] } })
    renderScreen({ row: { ...row, body_doc: null, body_html: "<p>legacy</p>" }, convert })
    await waitFor(() => expect(screen.getByTestId("ve-editor")).toBeTruthy())
    expect(convert).toHaveBeenCalledTimes(1)
    expect((screen.getByTestId("ve-save") as HTMLButtonElement).disabled).toBe(false)
  })

  it("offers a way back when conversion fails", async () => {
    const convert = vi.fn().mockRejectedValue(new Error("nope"))
    const props = renderScreen({ row: { ...row, body_doc: null, body_html: "<p>legacy</p>" }, convert })
    expect(await screen.findByTestId("ve-convert-failed")).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Back" }))
    expect(props.onBack).toHaveBeenCalled()
  })

  it("shows the empty state for an empty doc and opens the library from it", async () => {
    renderScreen({ row: { ...row, body_doc: { version: 1, type: "doc", content: [{ type: "paragraph", content: [] }] } } })
    fireEvent.click(await screen.findByTestId("ve-empty-add"))
    expect(await screen.findByTestId("ve-library")).toBeTruthy()
  })

  it("moves the side panel into a sheet on a narrow viewport", async () => {
    mockMatchMedia(true)
    renderScreen()
    await waitFor(() => expect(screen.getByTestId("ve-open-panel")).toBeTruthy())
    expect(screen.queryByTestId("ve-params-panel")).toBeNull()
    fireEvent.click(screen.getByTestId("ve-open-panel"))
    expect(await screen.findByTestId("ve-params-panel")).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor/EditorScreen.test.tsx`
Expected: FAIL. Module not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/visual-editor/EditorScreen.tsx`:
```tsx
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
const NARROW_QUERY = "(max-width: 1024px)"

// useIsNarrow: too narrow for canvas + side panel side by side. Defaults to
// wide so no-matchMedia environments get the full layout.
function useIsNarrow(): boolean {
  const [narrow, setNarrow] = useState(false)
  useEffect(() => {
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

  // Cmd/Ctrl+S saves; Esc returns to the root «Email» unless an editor
  // plugin (e.g. the slash menu) already handled the key.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault()
        if (editable && session.dirty && !session.saving) session.save()
        return
      }
      if (e.key === "Escape" && !e.defaultPrevented) selection.clear()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [editable, session, selection])

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
            <SheetContent side="right" className="ve-overlay-panel">
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
          <ResizablePanel id="ve-main" defaultSize={74} minSize={40}>
            {canvas}
          </ResizablePanel>
          <ResizableHandle withHandle />
          <ResizablePanel id="ve-side-panel" defaultSize={26} minSize={18} maxSize={45} className="ve-screen__side">
            {side}
          </ResizablePanel>
        </ResizablePanelGroup>
      )}
      <BlockLibrary open={libraryOpen} onOpenChange={setLibraryOpen} onInsert={onInsert} />
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="flex h-[90vh] flex-col sm:max-w-5xl" data-testid="ve-preview-dialog">
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
      <ResizablePanel id="ve-layers" defaultSize={40} minSize={15} className="ve-side__pane">
        {editor ? <StructureOutline editor={editor} selection={selection} editable={editable} /> : null}
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel id="ve-settings" defaultSize={60} minSize={25} className="ve-side__pane">
        <BlockParamsPanel selection={selection} editor={editor} root={root} disabled={!editable} />
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}
```

If `useEditorState`'s return type is non-nullable in the installed version, `history?.` will fail lint (no-unnecessary-condition). In that case drop the `?.` and keep the `?? false` inside the selector.

- [ ] **Step 4: CSS**

Append to `visual-editor.css`:
```css
.ve-screen {
  display: flex;
  flex-direction: column;
  height: 100svh;
  background: #f4f4f5;
}

.ve-screen__body {
  flex: 1 1 auto;
  min-height: 0;
}

.ve-screen__side {
  border-left: 1px solid #e5e7eb;
  background: #fff;
}

.ve-screen__notice {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 48px;
  color: #6b7280;
}

/* Canvas: grey dotted backdrop with the white email page centered on it. */
.ve-canvas {
  height: 100%;
  overflow: auto;
  padding: 32px 24px 96px;
  background-color: #f4f4f5;
  background-image: radial-gradient(#d4d4d8 1px, transparent 1px);
  background-size: 16px 16px;
}

.ve-page {
  position: relative;
  max-width: 100%;
  margin: 0 auto;
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  transition: width 0.15s ease;
}

.ve-page .ve-editor {
  min-height: 480px;
  border: none;
  border-radius: 0;
}

/* Thin grey outline on hover; the blue selection outline only while a block
   is selected (the decoration always marks ProseMirror's internal selection). */
.ve-page .ve-editor > *:hover {
  outline: 1px solid #cbd5e1;
  outline-offset: 2px;
}

.ve-page .ve-editor .ve-selected {
  outline: none;
}

.ve-page.ve-has-selection .ve-editor .ve-selected {
  outline: 2px solid #2563eb;
  outline-offset: 2px;
}

.ve-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  margin: 0 16px 16px;
  padding: 32px;
  border: 1px dashed #d4d4d8;
  border-radius: 8px;
  color: #52525b;
  font-size: 14px;
}

.ve-side {
  height: 100%;
}

.ve-side__pane {
  overflow: auto;
}

.ve-side .ve-panel {
  background: #fff;
}
```

- [ ] **Step 5: Story**

Create `frontend/src/components/visual-editor/EditorScreen.stories.tsx`:
```tsx
import { fn } from "storybook/test"
import { EditorScreen } from "./EditorScreen"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: EditorScreen,
  parameters: { layout: "fullscreen" },
  args: {
    slug: "acme",
    section: "Templates",
    row: {
      id: "tpl-1",
      name: "Welcome email",
      body_doc: {
        version: 1,
        type: "doc",
        content: [
          { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "Hello!" }] },
          { type: "paragraph", content: [{ type: "text", text: "Thanks for joining." }] },
          { type: "button", attrs: { label: "Get started", href: "https://example.com" } },
        ],
      },
      body_html: "",
      theme: null,
      updated_at: "2026-01-01T00:00:00Z",
    },
    editable: true,
    onBack: fn(),
    convert: fn(),
    save: fn(async () => ({ data: { updatedAt: "2026-01-02T00:00:00Z", warnings: [] } })),
    refetch: fn(),
    onSaved: fn(async () => {}),
  },
} satisfies Meta<typeof EditorScreen>

export default meta
type Story = StoryObj<typeof meta>

export const Editing: Story = {}
export const ReadOnly: Story = { args: { editable: false } }
```
If Storybook complains that `useBlocker` needs a router, add a decorator that wraps the story in a memory router, following the pattern in `src/components/shell/app-shell.stories.tsx` (it builds `createRouter` + `createMemoryHistory` + `RouterProvider`).

- [ ] **Step 6: Run the tests**

Run: `.claude/scripts/test-frontend.sh src/components/visual-editor`
Expected: `EXIT:0`.

- [ ] **Step 7: Commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/EditorScreen.tsx src/components/visual-editor/EditorScreen.test.tsx src/components/visual-editor/EditorScreen.stories.tsx && pnpm typecheck && cd ..
git add frontend/src/components/visual-editor/EditorScreen.tsx frontend/src/components/visual-editor/EditorScreen.test.tsx frontend/src/components/visual-editor/EditorScreen.stories.tsx frontend/src/components/visual-editor/visual-editor.css
git commit -m "feat(visual-editor): full-screen EditorScreen with canvas, layers, and settings"
```

---

### Task 13: Full-screen edit routes

**Files:**
- Modify: `frontend/src/components/shell/app-shell.tsx`
- Create: `frontend/src/components/shell/app-shell.test.tsx`
- Modify: `frontend/src/routes/t/$slug/route.test.tsx` (router mock)
- Create: `frontend/src/routes/t/$slug/templates/$id_.edit.tsx`
- Create: `frontend/src/routes/t/$slug/templates/$id_.edit.test.tsx`
- Create: `frontend/src/routes/t/$slug/campaigns/$id_.edit.tsx`
- Create: `frontend/src/routes/t/$slug/campaigns/$id_.edit.test.tsx`
- Regenerate: `frontend/src/routeTree.gen.ts`

**Interfaces:**
- Consumes: `EditorScreen`, `EditorScreenProps` (Task 12); `api.getTemplate`, `api.templates.{saveVisual,convertToVisual}`, `api.getCampaign`, `api.campaigns.{saveVisual,convertToVisual}`; `queryKeys.{template,templates,campaign,campaigns}`; `usePermissions`; `AsyncState`.
- Produces: routes `/t/$slug/templates/$id/edit` and `/t/$slug/campaigns/$id/edit` with `staticData: { fullscreen: true }`; exported `TemplateEditPage`, `CampaignEditPage`.

- [ ] **Step 1: AppShell test (failing)**

Create `frontend/src/components/shell/app-shell.test.tsx`:
```tsx
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import { AppShell } from "./app-shell"

const state = vi.hoisted(() => ({ fullscreen: true }))

vi.mock("@tanstack/react-router", () => ({
  useMatches: ({ select }: { select: (m: Array<{ staticData: { fullscreen?: boolean } }>) => boolean }) =>
    select([{ staticData: { fullscreen: state.fullscreen } }]),
}))
vi.mock("@/hooks/use-locale", () => ({ useSyncAccountLocale: vi.fn() }))
vi.mock("./sidebar", () => ({ WorkspaceSidebar: () => <nav data-testid="sidebar" /> }))
vi.mock("./top-bar", () => ({ TopBar: () => <header data-testid="top-bar" /> }))
vi.mock("./suspension-banner", () => ({ SuspensionBanner: () => null }))

afterEach(() => cleanup())

describe("AppShell", () => {
  it("renders only the page for fullscreen routes", () => {
    state.fullscreen = true
    render(
      <AppShell slug="acme" workspaceName="Acme">
        <div data-testid="page" />
      </AppShell>,
    )
    expect(screen.getByTestId("page")).toBeTruthy()
    expect(screen.queryByTestId("sidebar")).toBeNull()
    expect(screen.queryByTestId("top-bar")).toBeNull()
  })

  it("wraps other routes in the sidebar and top bar", () => {
    state.fullscreen = false
    render(
      <AppShell slug="acme" workspaceName="Acme">
        <div data-testid="page" />
      </AppShell>,
    )
    expect(screen.getByTestId("sidebar")).toBeTruthy()
    expect(screen.getByTestId("top-bar")).toBeTruthy()
  })
})
```

Run: `.claude/scripts/test-frontend.sh src/components/shell/app-shell.test.tsx`
Expected: FAIL. The first test finds the sidebar.

- [ ] **Step 2: Implement fullscreen in AppShell**

In `frontend/src/components/shell/app-shell.tsx`:

```tsx
import { useMatches } from "@tanstack/react-router"
// …existing imports…

// Routes opt out of the workspace chrome with `staticData: { fullscreen: true }`
// (the visual email editor). Session/TOTP gating in /t/$slug still applies.
declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    fullscreen?: boolean
  }
}

export function AppShell({ slug, workspaceName, children }: AppShellProps) {
  // Apply the signed-in user's stored language preference (FR-004).
  useSyncAccountLocale()
  const fullscreen = useMatches({
    select: (matches) => matches.some((m) => m.staticData.fullscreen === true),
  })

  if (fullscreen) return <>{children}</>

  return (
    // …unchanged SidebarProvider tree…
  )
}
```

In `frontend/src/routes/t/$slug/route.test.tsx`, add this to the `vi.mock("@tanstack/react-router", …)` factory object:
```ts
  useMatches: ({ select }: { select: (m: Array<unknown>) => unknown }) => select([]),
```

Run: `.claude/scripts/test-frontend.sh src/components/shell 'src/routes/t/$slug/route.test.tsx'`
Expected: `EXIT:0`.

- [ ] **Step 3: Template edit route test (failing)**

Create `frontend/src/routes/t/$slug/templates/$id_.edit.test.tsx`:
```tsx
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { TemplateEditPage } from "./$id_.edit"
import type { VisualDoc } from "@/lib/api-types"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => ({
    ...opts,
    useParams: () => ({ slug: "acme", id: "tpl-1" }),
  }),
  useNavigate: () => navigate,
  useBlocker: vi.fn(),
}))

vi.mock("@/lib/api", () => ({
  api: {
    getTemplate: vi.fn(),
    me: vi.fn(),
    tenant: vi.fn(),
    listRoles: vi.fn(),
    branding: { get: vi.fn() },
    templates: { saveVisual: vi.fn(), convertToVisual: vi.fn() },
  },
}))

// The TipTap canvas is covered by EditorScreen/VisualEmailEditor tests; here
// a stub exposes the doc size and an "edit" button that makes the doc dirty.
vi.mock("@/components/visual-editor/VisualEmailEditor", () => ({
  VisualEmailEditor: ({ value, onChange }: { value: VisualDoc; onChange: (d: VisualDoc) => void }) => (
    <div data-testid="visual-email-editor" data-doc-blocks={String(value.content.length)}>
      <button
        type="button"
        data-testid="stub-edit"
        onClick={() => onChange({ ...value, content: [...value.content, { type: "divider" }] })}
      >
        edit
      </button>
    </div>
  ),
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const visualDoc: VisualDoc = {
  version: 1,
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "Hi" }] }],
}

function template(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "tpl-1",
    name: "Welcome",
    kind: "campaign" as const,
    subject: "Hi there",
    body_html: "<p>Hi</p>",
    body_text: "Hi",
    body_doc: visualDoc,
    theme: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }
}

function setupOwner() {
  vi.mocked(api.me).mockResolvedValue(
    ok({ user: { id: "u1", name: "Ann", email: "ann@ex.com", locale: null }, tenants: [] }),
  )
  vi.mocked(api.tenant).mockResolvedValue(
    ok({ tenant: { name: "Acme" }, members: [{ user_id: "u1", email: "ann@ex.com", name: "Ann", role: "Owner" }] }),
  )
  vi.mocked(api.listRoles).mockResolvedValue(ok({ roles: [] }))
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("TemplateEditPage", () => {
  it("opens the template in the full-screen editor", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template()))
    renderWithClient(<TemplateEditPage />)
    expect((await screen.findByTestId("ve-topbar-name")).textContent).toBe("Welcome")
    expect(screen.getByTestId("visual-email-editor").getAttribute("data-doc-blocks")).toBe("1")
  })

  it("saves through templates.saveVisual with name, kind, subject and the row token", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template()))
    vi.mocked(api.templates.saveVisual).mockResolvedValue(
      ok({ updatedAt: "2026-01-02T00:00:00Z", warnings: [] }) as never,
    )
    renderWithClient(<TemplateEditPage />)
    fireEvent.click(await screen.findByTestId("stub-edit"))
    await waitFor(() => expect((screen.getByTestId("ve-save") as HTMLButtonElement).disabled).toBe(false))
    fireEvent.click(screen.getByTestId("ve-save"))
    await waitFor(() =>
      expect(api.templates.saveVisual).toHaveBeenCalledWith("acme", "tpl-1", {
        name: "Welcome",
        kind: "campaign",
        subject: "Hi there",
        bodyDoc: { ...visualDoc, content: [...visualDoc.content, { type: "divider" }] },
        theme: null,
        ifUnmodifiedSince: "2026-01-01T00:00:00Z",
      }),
    )
  })

  it("converts a legacy raw-HTML template on load", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: null, body_html: "<p>legacy</p>" })))
    vi.mocked(api.templates.convertToVisual).mockResolvedValue(ok({ bodyDoc: visualDoc, warnings: [] }))
    renderWithClient(<TemplateEditPage />)
    await screen.findByTestId("visual-email-editor")
    expect(api.templates.convertToVisual).toHaveBeenCalledWith("acme", "tpl-1")
  })

  it("refuses transactional templates", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ kind: "transactional" })))
    renderWithClient(<TemplateEditPage />)
    expect(await screen.findByText(/not available for transactional templates/i)).toBeTruthy()
    expect(screen.queryByTestId("visual-email-editor")).toBeNull()
  })

  it("goes back to the template detail page", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template()))
    renderWithClient(<TemplateEditPage />)
    fireEvent.click(await screen.findByTestId("ve-back"))
    expect(navigate).toHaveBeenCalledWith({ to: "/t/$slug/templates/$id", params: { slug: "acme", id: "tpl-1" } })
  })
})
```

Run: `.claude/scripts/test-frontend.sh 'src/routes/t/$slug/templates/$id_.edit.test.tsx'`
Expected: FAIL. Module not found.

- [ ] **Step 4: Implement the template edit route**

Create `frontend/src/routes/t/$slug/templates/$id_.edit.tsx`:
```tsx
// Full-screen visual editor for a template (/t/$slug/templates/$id/edit). The
// trailing underscore keeps the route out of $id.tsx's tree (that page has no
// <Outlet/>). AppShell hides its chrome via staticData.fullscreen.
// Transactional templates stay code-only, so they never open here.

import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { usePermissions } from "@/hooks/use-permissions"
import { AsyncState } from "@/components/common/async-state"
import { Button } from "@/components/ui/button"
import { EditorScreen } from "@/components/visual-editor/EditorScreen"

export const Route = createFileRoute("/t/$slug/templates/$id_/edit")({
  component: TemplateEditPage,
  staticData: { fullscreen: true },
})

export function TemplateEditPage() {
  const { slug, id } = Route.useParams()
  const { t } = useTranslation("visualEditor")
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)

  const query = useQuery({
    queryKey: queryKeys.template(slug, id),
    queryFn: async () => (await api.getTemplate(slug, id)).data,
  })

  const onBack = () => {
    void navigate({ to: "/t/$slug/templates/$id", params: { slug, id } })
  }

  return (
    <AsyncState query={query}>
      {(tpl) =>
        tpl.kind === "transactional" ? (
          <div className="ve-screen__notice">
            <p>{t("screen.unavailable")}</p>
            <Button type="button" variant="outline" onClick={onBack}>
              {t("screen.back")}
            </Button>
          </div>
        ) : (
          <EditorScreen
            key={tpl.id}
            slug={slug}
            section={t("screen.breadcrumb.templates")}
            row={tpl}
            editable={can("campaigns:manage")}
            onBack={onBack}
            convert={() => api.templates.convertToVisual(slug, id)}
            save={(v) =>
              api.templates.saveVisual(slug, id, {
                name: tpl.name,
                kind: tpl.kind,
                subject: tpl.subject,
                ...v,
              })
            }
            refetch={async () => (await api.getTemplate(slug, id)).data}
            onSaved={async () => {
              await queryClient.invalidateQueries({ queryKey: queryKeys.templates(slug) })
              await queryClient.invalidateQueries({ queryKey: queryKeys.template(slug, id) })
            }}
          />
        )
      }
    </AsyncState>
  )
}
```

The `kind` check matters: `TemplateView.kind` is `"campaign" | "transactional"`.

- [ ] **Step 5: Campaign edit route test + implementation**

Create `frontend/src/routes/t/$slug/campaigns/$id_.edit.test.tsx`. Use the same scaffolding as Step 3, with these differences:
- `useParams` returns `{ slug: "acme", id: "camp-1" }`; import `CampaignEditPage` from `"./$id_.edit"`.
- The `api` mock has `getCampaign` instead of `getTemplate`, and `campaigns: { saveVisual: vi.fn(), convertToVisual: vi.fn() }`.
- Fixture:
```ts
function campaign(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "camp-1",
    name: "Spring Sale",
    subject: "Big news",
    body_html: "<p>Hi</p>",
    body_text: "Hi",
    body_doc: visualDoc,
    theme: null,
    status: "draft",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }
}
```
- Tests:
```tsx
describe("CampaignEditPage", () => {
  it("saves a draft through campaigns.saveVisual with the subject and row token", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()) as never)
    vi.mocked(api.campaigns.saveVisual).mockResolvedValue(
      ok({ updatedAt: "2026-01-02T00:00:00Z", warnings: [] }) as never,
    )
    renderWithClient(<CampaignEditPage />)
    fireEvent.click(await screen.findByTestId("stub-edit"))
    await waitFor(() => expect((screen.getByTestId("ve-save") as HTMLButtonElement).disabled).toBe(false))
    fireEvent.click(screen.getByTestId("ve-save"))
    await waitFor(() =>
      expect(api.campaigns.saveVisual).toHaveBeenCalledWith("acme", "camp-1", {
        subject: "Big news",
        bodyDoc: { ...visualDoc, content: [...visualDoc.content, { type: "divider" }] },
        theme: null,
        ifUnmodifiedSince: "2026-01-01T00:00:00Z",
      }),
    )
  })

  it("opens a non-draft campaign read-only", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign({ status: "sent" })) as never)
    renderWithClient(<CampaignEditPage />)
    expect(await screen.findByTestId("ve-readonly-badge")).toBeTruthy()
    expect(screen.queryByTestId("ve-save")).toBeNull()
  })

  it("goes back to the campaign detail page", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign()) as never)
    renderWithClient(<CampaignEditPage />)
    fireEvent.click(await screen.findByTestId("ve-back"))
    expect(navigate).toHaveBeenCalledWith({ to: "/t/$slug/campaigns/$id", params: { slug: "acme", id: "camp-1" } })
  })
})
```

Create `frontend/src/routes/t/$slug/campaigns/$id_.edit.tsx`:
```tsx
// Full-screen visual editor for a campaign (/t/$slug/campaigns/$id/edit).
// Only drafts are editable; any other status opens read-only. The trailing
// underscore keeps the route out of $id.tsx's tree.

import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { usePermissions } from "@/hooks/use-permissions"
import { AsyncState } from "@/components/common/async-state"
import { EditorScreen } from "@/components/visual-editor/EditorScreen"

export const Route = createFileRoute("/t/$slug/campaigns/$id_/edit")({
  component: CampaignEditPage,
  staticData: { fullscreen: true },
})

export function CampaignEditPage() {
  const { slug, id } = Route.useParams()
  const { t } = useTranslation("visualEditor")
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)

  const query = useQuery({
    queryKey: queryKeys.campaign(slug, id),
    queryFn: async () => (await api.getCampaign(slug, id)).data,
  })

  return (
    <AsyncState query={query}>
      {(campaign) => (
        <EditorScreen
          key={campaign.id}
          slug={slug}
          section={t("screen.breadcrumb.campaigns")}
          row={campaign}
          editable={can("campaigns:manage") && campaign.status === "draft"}
          onBack={() => {
            void navigate({ to: "/t/$slug/campaigns/$id", params: { slug, id } })
          }}
          convert={() => api.campaigns.convertToVisual(slug, id)}
          save={(v) => api.campaigns.saveVisual(slug, id, { subject: campaign.subject, ...v })}
          refetch={async () => (await api.getCampaign(slug, id)).data}
          onSaved={async () => {
            await queryClient.invalidateQueries({ queryKey: queryKeys.campaign(slug, id) })
            await queryClient.invalidateQueries({ queryKey: queryKeys.campaigns(slug) })
          }}
        />
      )}
    </AsyncState>
  )
}
```

- [ ] **Step 6: Regenerate the route tree**

Run: `cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm build && git diff --stat src/routeTree.gen.ts && cd ..`
Expected: `src/routeTree.gen.ts` gains the two edit routes. If the generator rewrites the `createFileRoute("…")` strings in the new files, keep its version.

- [ ] **Step 7: Run tests, typecheck, commit**

Run: `.claude/scripts/test-frontend.sh src/components/shell 'src/routes/t/$slug'`
Expected: `EXIT:0`.

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/shell 'src/routes/t/$slug/templates/$id_.edit.tsx' 'src/routes/t/$slug/templates/$id_.edit.test.tsx' 'src/routes/t/$slug/campaigns/$id_.edit.tsx' 'src/routes/t/$slug/campaigns/$id_.edit.test.tsx' && pnpm typecheck && cd ..
git add frontend/src/components/shell frontend/src/routeTree.gen.ts 'frontend/src/routes/t/$slug/route.test.tsx' 'frontend/src/routes/t/$slug/templates/$id_.edit.tsx' 'frontend/src/routes/t/$slug/templates/$id_.edit.test.tsx' 'frontend/src/routes/t/$slug/campaigns/$id_.edit.tsx' 'frontend/src/routes/t/$slug/campaigns/$id_.edit.test.tsx'
git commit -m "feat(visual-editor): full-screen edit routes for templates and campaigns"
```

---

### Task 14: Detail pages — preview + «Open editor» instead of the inline editor

**Files:**
- Create: `frontend/src/components/visual-editor/VisualBodyPreview.tsx`
- Modify: `frontend/src/routes/t/$slug/templates/$id.tsx`
- Modify: `frontend/src/routes/t/$slug/templates/$id.test.tsx`
- Modify: `frontend/src/routes/t/$slug/campaigns/$id.tsx`
- Modify: `frontend/src/routes/t/$slug/campaigns/$id.test.tsx`

**Interfaces:**
- Produces: `VisualBodyPreview({ html: string; actions?: ReactNode })`, test id `ve-body-preview`. Detail pages expose links with test ids `open-visual-editor` and `convert-to-visual` (both point at `…/$id/edit`) and the button `ve-opt-out-visual`.

- [ ] **Step 1: Rewrite the template detail tests (failing)**

In `frontend/src/routes/t/$slug/templates/$id.test.tsx`:

1. Make the router mock forward `data-testid`:
```tsx
  Link: ({ children, to, "data-testid": testId }: { children: unknown; to?: unknown; "data-testid"?: string }) => (
    <a href={typeof to === "string" ? to : "#"} data-testid={testId}>
      {children as never}
    </a>
  ),
```
2. Delete the `vi.mock("@/components/visual-editor/VisualEmailEditor", …)` block (the detail page no longer mounts the editor).
3. Replace the describe blocks `"TemplateDetail — visual editor surface (T082)"` and `"TemplateDetail — convert / opt-out (T094)"` with:
```tsx
describe("TemplateDetail — visual body", () => {
  it("shows the saved-body preview and an Open editor link for a visual row", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: visualDoc })))
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByTestId("ve-body-preview")).toBeTruthy()
    expect(screen.getByTestId("open-visual-editor").getAttribute("href")).toBe("/t/$slug/templates/$id/edit")
    expect(screen.queryByText(/html body/i)).toBeNull()
  })

  it("keeps the code editor for a legacy raw-HTML template and links Convert to the editor", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: null, body_html: "<p>legacy</p>" })))
    renderWithClient(<TemplateDetail />)
    expect(await screen.findByText(/html body/i)).toBeTruthy()
    expect(screen.getByTestId("convert-to-visual").getAttribute("href")).toBe("/t/$slug/templates/$id/edit")
    expect(api.templates.convertToVisual).not.toHaveBeenCalled()
  })

  it("hides Convert on transactional templates", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ kind: "transactional", body_doc: null })))
    renderWithClient(<TemplateDetail />)
    await screen.findByRole("button", { name: /save changes/i })
    expect(screen.queryByTestId("convert-to-visual")).toBeNull()
  })

  it("saves only name and subject from the detail page in visual mode", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: visualDoc, body_html: "<p>kept</p>" })))
    vi.mocked(api.updateTemplate).mockResolvedValue(ok(template()))
    renderWithClient(<TemplateDetail />)
    const nameInput = await screen.findByLabelText(/name/i)
    fireEvent.change(nameInput, { target: { value: "Welcome v2" } })
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }))
    await waitFor(() =>
      expect(api.updateTemplate).toHaveBeenCalledWith(
        "acme",
        "tpl-1",
        expect.objectContaining({ name: "Welcome v2", body_html: "<p>kept</p>" }),
      ),
    )
    expect(api.templates.saveVisual).not.toHaveBeenCalled()
  })

  it("opts out of the visual editor from the detail page", async () => {
    setupOwner()
    vi.mocked(api.getTemplate).mockResolvedValue(ok(template({ body_doc: visualDoc, body_html: "<p>kept</p>" })))
    vi.mocked(api.templates.optOutVisual).mockResolvedValue(ok(template({ body_doc: null, body_html: "<p>kept</p>" })))
    renderWithClient(<TemplateDetail />)
    fireEvent.click(await screen.findByTestId("ve-opt-out-visual"))
    fireEvent.click(await screen.findByTestId("opt-out-visual-confirm"))
    await waitFor(() => expect(api.templates.optOutVisual).toHaveBeenCalledWith("acme", "tpl-1"))
  })
})
```
4. In the `"transactional templates (T084)"` describe, replace each `expect(screen.queryByTestId("visual-email-editor")).toBeNull()` with `expect(screen.queryByTestId("open-visual-editor")).toBeNull()`.

Run: `.claude/scripts/test-frontend.sh 'src/routes/t/$slug/templates/$id.test.tsx'`
Expected: FAIL. There is no `ve-body-preview` yet.

- [ ] **Step 2: Create `VisualBodyPreview`**

Create `frontend/src/components/visual-editor/VisualBodyPreview.tsx`:
```tsx
// VisualBodyPreview — the detail-page stand-in for the visual editor: a
// sandboxed preview of the saved body_html plus caller-supplied actions
// ("Open editor", "Edit as HTML only"). It shows the stored HTML as-is, so it
// costs no render call.

import { useTranslation } from "react-i18next"
import type { ReactNode } from "react"

export function VisualBodyPreview({ html, actions }: { html: string; actions?: ReactNode }) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="flex flex-col gap-2" data-testid="ve-body-preview">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{t("body.previewTitle")}</span>
        <div className="flex items-center gap-2">{actions}</div>
      </div>
      {html.trim() === "" ? (
        <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t("body.previewEmpty")}
        </p>
      ) : (
        <iframe
          title={t("body.previewTitle")}
          sandbox=""
          srcDoc={html}
          className="h-96 w-full rounded-md border bg-white"
        />
      )}
    </div>
  )
}
```

- [ ] **Step 3: Rework the template detail page**

In `frontend/src/routes/t/$slug/templates/$id.tsx`:

1. Imports: `import type { TemplateView } from "@/lib/api-types"`; `import { errorMessage } from "@/lib/errors"`; remove `import { ThreePaneEditor } …`; add `import { VisualBodyPreview } from "@/components/visual-editor/VisualBodyPreview"`.
2. Delete `EMPTY_VISUAL_DOC`, the `bodyDoc` / `themeOverride` / `ifUnmodifiedSince` state, `convertToVisual`, `refetchTemplate`, and `saveVisual` (with their comments).
3. `const { t } = useTranslation(["templates", "common", "visualEditor"])`.
4. Replace the form's `onSubmit` with:
```ts
    onSubmit: async ({ value }) => {
      // In visual mode the body belongs to the full-screen editor; this form
      // saves name + subject only and sends the stored body back unchanged
      // (the legacy update never touches body_doc).
      const body =
        editorMode === "visual"
          ? { bodyHtml: template.body_html, bodyText: template.body_text }
          : {}
      await save.mutateAsync({ ...value, ...body }).catch(() => {})
    },
```
5. Replace the `{editorMode === "visual" && canManage ? ( <div …><Label …/><ThreePaneEditor …/></div> ) : (` opening with:
```tsx
          {editorMode === "visual" ? (
            <VisualBodyPreview
              html={template.body_html}
              actions={
                canManage && (
                  <>
                    <Button asChild size="sm">
                      <Link
                        to="/t/$slug/templates/$id/edit"
                        params={{ slug, id: template.id }}
                        data-testid="open-visual-editor"
                      >
                        {t("visualEditor:body.openEditor")}
                      </Link>
                    </Button>
                    {template.body_doc && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setConfirmOptOut(true)}
                        data-testid="ve-opt-out-visual"
                      >
                        {t("visualEditor:body.editHtmlOnly")}
                      </Button>
                    )}
                  </>
                )
              }
            />
          ) : (
```
6. Replace the convert `<Button … onClick={() => convertToVisual.mutate()} data-testid="convert-to-visual">…</Button>` with:
```tsx
                          <Button asChild variant="outline" size="sm">
                            <Link
                              to="/t/$slug/templates/$id/edit"
                              params={{ slug, id: template.id }}
                              data-testid="convert-to-visual"
                            >
                              {t("visual.convert")}
                            </Link>
                          </Button>
```
7. Submit button: `disabled={save.isPending}` and `{save.isPending ? t("detail.saving") : t("detail.save")}`.
8. Update `initialTemplateEditorMode`'s comment: visual rows show a preview and link to the full-screen editor.

Run: `.claude/scripts/test-frontend.sh 'src/routes/t/$slug/templates/$id.test.tsx'`
Expected: `EXIT:0`.

- [ ] **Step 4: Same change for campaigns — tests first**

In `frontend/src/routes/t/$slug/campaigns/$id.test.tsx`:
1. Make the `Link` mock forward `data-testid` (same snippet as Step 1, item 1).
2. Delete the `VisualEmailEditor` module mock.
3. In `describe("CampaignDetail — visual editor surface (T070, T127)")`, keep the `visualDoc` const and replace the tests with:
```tsx
  it("shows the saved-body preview and an Open editor link for a visual row", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign({ body_doc: visualDoc })))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)
    expect(await screen.findByTestId("ve-body-preview")).toBeTruthy()
    expect(screen.getByTestId("open-visual-editor").getAttribute("href")).toBe("/t/$slug/campaigns/$id/edit")
    expect(screen.queryByTestId("open-media-picker")).toBeNull()
  })

  it("keeps the code editor for a legacy raw-HTML row and links Convert to the editor", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign({ body_doc: null, body_html: "<p>legacy</p>" })))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    renderWithClient(<CampaignDetail />)
    expect(await screen.findByTestId("open-media-picker")).toBeTruthy()
    expect(screen.getByTestId("convert-to-visual").getAttribute("href")).toBe("/t/$slug/campaigns/$id/edit")
    expect(api.campaigns.convertToVisual).not.toHaveBeenCalled()
  })

  it("saves only the form fields from the detail page in visual mode", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign({ body_doc: visualDoc, body_html: "<p>kept</p>" })))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.updateCampaign).mockResolvedValue(ok(campaign()))
    renderWithClient(<CampaignDetail />)
    await screen.findByTestId("ve-body-preview")
    fireEvent.click(screen.getByRole("button", { name: /save/i }))
    await waitFor(() =>
      expect(api.updateCampaign).toHaveBeenCalledWith(
        "acme",
        "camp-1",
        expect.objectContaining({ body_html: "<p>kept</p>" }),
      ),
    )
    expect(api.campaigns.saveVisual).not.toHaveBeenCalled()
  })

  it("opts out of the visual editor from the detail page", async () => {
    setupOwner()
    vi.mocked(api.getCampaign).mockResolvedValue(ok(campaign({ body_doc: visualDoc, body_html: "<p>kept</p>" })))
    vi.mocked(api.listSendingDomains).mockResolvedValue(ok({ domains: [] }))
    vi.mocked(api.campaigns.optOutVisual).mockResolvedValue(ok(campaign({ body_doc: null, body_html: "<p>kept</p>" })))
    renderWithClient(<CampaignDetail />)
    fireEvent.click(await screen.findByTestId("ve-opt-out-visual"))
    fireEvent.click(await screen.findByTestId("opt-out-visual-confirm"))
    await waitFor(() => expect(api.campaigns.optOutVisual).toHaveBeenCalledWith("acme", "camp-1"))
  })
```
If the campaign page's submit button label isn't matched by `/save/i`, copy the selector used by the existing `"CampaignDetail"` tests.

Run: `.claude/scripts/test-frontend.sh 'src/routes/t/$slug/campaigns/$id.test.tsx'`
Expected: FAIL. There is no `ve-body-preview` yet.

- [ ] **Step 5: Rework the campaign detail page**

In `frontend/src/routes/t/$slug/campaigns/$id.tsx`:
1. Remove `import { ThreePaneEditor } …`; add `import { VisualBodyPreview } from "@/components/visual-editor/VisualBodyPreview"`. Drop `Theme` and `VisualDoc` from the `import type {…}` if they become unused. **Keep** the `ApiError` import (the start-campaign errors use it).
2. Delete `EMPTY_VISUAL_DOC`, the `bodyDoc` / `themeOverride` / `ifUnmodifiedSince` state, `refetchCampaign`, `saveVisual`, and `convertToVisual`.
3. Add `"visualEditor"` to the component's `useTranslation([...])` namespaces.
4. Replace the form's `onSubmit` with:
```ts
    onSubmit: async ({ value }) => {
      // In visual mode the body belongs to the full-screen editor; this form
      // saves name, sender and recipients and sends the stored body back
      // unchanged (the legacy update never touches body_doc).
      const body =
        editorMode === "visual"
          ? { bodyHtml: campaign.body_html, bodyText: campaign.body_text }
          : {}
      await save.mutateAsync({ ...value, ...body }).catch(() => {})
    },
```
5. Replace the `{editorMode === "visual" && canManage ? ( <div …><Label …/><ThreePaneEditor …/></div> ) : (` opening with the Step 3 item 5 snippet, substituting `campaign` for `template` and `to="/t/$slug/campaigns/$id/edit"`.
6. Replace the convert button with the Step 3 item 6 snippet, using `to="/t/$slug/campaigns/$id/edit"` and `params={{ slug, id: campaign.id }}`.
7. Replace every `save.isPending || saveVisual.isPending` with `save.isPending`.

Run: `.claude/scripts/test-frontend.sh 'src/routes/t/$slug'`
Expected: `EXIT:0`.

- [ ] **Step 6: Typecheck, lint, commit**

```bash
cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm exec eslint --fix src/components/visual-editor/VisualBodyPreview.tsx 'src/routes/t/$slug/templates/$id.tsx' 'src/routes/t/$slug/templates/$id.test.tsx' 'src/routes/t/$slug/campaigns/$id.tsx' 'src/routes/t/$slug/campaigns/$id.test.tsx' && pnpm typecheck && cd ..
git add frontend/src/components/visual-editor/VisualBodyPreview.tsx 'frontend/src/routes/t/$slug/templates/$id.tsx' 'frontend/src/routes/t/$slug/templates/$id.test.tsx' 'frontend/src/routes/t/$slug/campaigns/$id.tsx' 'frontend/src/routes/t/$slug/campaigns/$id.test.tsx'
git commit -m "feat(visual-editor): detail pages preview the body and link to the full-screen editor"
```

---

### Task 15: Remove the dead three-pane shell and verify everything

**Files:**
- Delete: `frontend/src/components/visual-editor/ThreePaneEditor.tsx`, `ThreePaneEditor.test.tsx`, `ThreePaneEditor.stories.tsx`
- Modify: `frontend/src/components/visual-editor/VisualEmailEditor.tsx`
- Modify: `frontend/src/components/visual-editor/visual-editor.css`
- Modify: `frontend/src/locales/{en,ru}/visualEditor.json`, `frontend/src/i18n/resources.d.ts`

**Interfaces:**
- Produces: `VisualEmailEditor` props are now `{ slug, value, onChange, placeholder?, editable?, theme?, onEditorReady? }`.

- [ ] **Step 1: Confirm nothing uses the dead code**

Run: `grep -rn "ThreePaneEditor\|onSwitchToCodeView\|onOptOutVisual\|onThemeChange\|toolbar\.viewHtml\|toolbar\.editHtmlOnly\|panel\.collapse\|panel\.expand" frontend/src --include='*.ts' --include='*.tsx' | grep -v "visual-editor/ThreePaneEditor" | grep -v "visual-editor/VisualEmailEditor.tsx"`
Expected: no output. If anything is listed, finish the earlier task that should have removed it.

- [ ] **Step 2: Delete the shell and trim `VisualEmailEditor`**

```bash
git rm frontend/src/components/visual-editor/ThreePaneEditor.tsx frontend/src/components/visual-editor/ThreePaneEditor.test.tsx frontend/src/components/visual-editor/ThreePaneEditor.stories.tsx
```

In `VisualEmailEditor.tsx`:
- Remove the `onSwitchToCodeView`, `onOptOutVisual` and `onThemeChange` props (type members, their comments, and destructuring).
- Remove `const showToolbar = …` and the `{showToolbar && (<div className="ve-toolbar" …>…</div>)}` block.
- Remove the `{onThemeChange && (<ThemeControls …/>)}` block and the `ThemeControls` import.
- Keep `theme` (it still drives the canvas CSS variables) and update its comment: "Theme override for the canvas CSS variables; null = tenant branding. The theme panel itself lives in the editor screen's right panel."
- Update the header comment: the component is the canvas of `EditorScreen`.

- [ ] **Step 3: Remove dead CSS and strings**

In `visual-editor.css`, delete the rules whose selectors start with `.ve-three-pane`, `.ve-pane`, and `.ve-toolbar` (both shell sections, "Three-pane editor shell (feature 017)" and "Three-pane chrome"). Keep `.ve-overlay-panel` (the narrow sheet uses it) and every `.ve-panel*` rule.

In both `en` and `ru` `visualEditor.json`, delete the `toolbar` object and the `panel.collapseLeft`, `panel.collapseRight`, `panel.expandLeft`, `panel.expandRight` keys. Then:

Run: `cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm i18n:types && cd ..`

- [ ] **Step 4: Full verification**

Run: `cd frontend && export PATH="$(echo $HOME/.nvm/versions/node/*/bin):$PATH" && pnpm typecheck && pnpm lint && pnpm i18n:lint; cd ..`
Expected: all three exit 0.

Run: `.claude/scripts/test-frontend.sh`
Expected: `EXIT:0` (whole frontend suite, unit + Storybook).

- [ ] **Step 5: Manual check in the running app**

Use the `run` skill (or `cd frontend && pnpm dev` plus the Go API, per `specs/017-three-pane-visual-editor/quickstart.md`) and verify:
1. Template detail → «Open editor» opens the full-screen editor with no sidebar or app top bar.
2. «Add block» → library → Heading → H2 inserts after the selected block, and the floating label shows «Heading 2».
3. Esc → the Layers root «Email» is highlighted and the Design tab shows the theme controls.
4. Button → Content edits the label; Design → «Rounded corners» switch → radius 12 applies live.
5. Mobile toggle narrows the page to 375px; «Preview» shows server-rendered HTML.
6. Edit, then «←» → the unsaved-changes confirm appears. Save (⌘S), then «←» → no confirm.
7. Legacy raw-HTML campaign → «Convert to visual editor» opens the editor already dirty with a conversion toast.
8. A sent campaign opens read-only.

- [ ] **Step 6: Commit**

```bash
git add -A frontend/src/components/visual-editor frontend/src/locales frontend/src/i18n/resources.d.ts
git commit -m "refactor(visual-editor): drop the inline three-pane shell and unused editor props"
```

---

## Self-Review Notes

- **Spec coverage:** entry/routing (Tasks 13–14); top bar incl. undo/redo, device, preview, save, ⌘S (11–12); canvas, empty state, outlines, floating toolbar, Esc (2, 9, 12); right panel with Layers root, ⋮ menu, tabs, theme at root, grouped design controls, narrow sheet (5–7, 12); library + shared catalog + i18n slash labels (3, 8); errors: load via AsyncState, save/409 incl. force-overwrite fix, unsaved guard, preview error (PreviewIframe's own error state), read-only (10, 12–13); legacy conversion in the editor (12–13); tests and stories per component (every task); dead-code removal (15).
- **Out of scope by design:** drag from the library to the canvas, tablet viewport, hide-block, "save to my sections" (phases 2–5 of the spec).

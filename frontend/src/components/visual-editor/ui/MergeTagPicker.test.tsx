// MergeTagPicker tests (T069): renders the merged subscriber + custom +
// campaign-namespace list, filters by display name / slug, and inserts
// the selected entry into the supplied editor via the canonical
// `mergeTag` JSON node.

import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react"
import { useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { MergeTag } from "../extensions/MergeTag"
import { MergeTagPicker } from "./MergeTagPicker"
import type { Editor } from "@tiptap/core"
import { renderWithClient } from "@/test/render"
import { api } from "@/lib/api"

vi.mock("@/lib/api", () => ({
  api: {
    mergeTags: {
      list: vi.fn(),
    },
  },
}))

beforeAll(() => {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = () =>
      new DOMRect(0, 0, 0, 0)
  }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const ok = <T,>(data: T) => ({ status: 200, ok: true as const, data })

const sampleResponse = {
  subscriber: [
    {
      slug: "first_name",
      displayName: "First name",
      type: "text" as const,
      builtIn: true,
    },
    {
      slug: "email",
      displayName: "Email",
      type: "url" as const,
      builtIn: true,
    },
    {
      slug: "country",
      displayName: "Country",
      type: "text" as const,
      builtIn: false,
    },
  ],
  campaign: [
    { key: "unsubscribe_url", displayName: "Unsubscribe URL" },
    { key: "tenant_name", displayName: "Tenant name" },
  ],
}

// Headless harness — exposes the editor to the test so we can read back
// the JSON after the picker inserts a node.
function Harness({
  onEditor,
  open,
}: {
  onEditor: (e: Editor) => void
  open: boolean
}) {
  const editor = useEditor({
    extensions: [StarterKit.configure({ hardBreak: false }), MergeTag],
    content: { type: "doc", content: [{ type: "paragraph" }] },
    onCreate({ editor: ed }) {
      onEditor(ed)
    },
  })
  return (
    <MergeTagPicker
      slug="acme"
      editor={editor}
      open={open}
    />
  )
}

describe("MergeTagPicker", () => {
  it("lists built-in + custom + campaign-namespace entries", async () => {
    vi.mocked(api.mergeTags.list).mockResolvedValue(ok(sampleResponse))
    renderWithClient(<Harness onEditor={() => {}} open={true} />)
    await waitFor(() => {
      expect(screen.getByText("First name")).toBeTruthy()
    })
    expect(screen.getByText("Email")).toBeTruthy()
    expect(screen.getByText("Country")).toBeTruthy()
    expect(screen.getByText("Unsubscribe URL")).toBeTruthy()
    expect(screen.getByText("Tenant name")).toBeTruthy()
  })

  it("filters as the operator types", async () => {
    vi.mocked(api.mergeTags.list).mockResolvedValue(ok(sampleResponse))
    renderWithClient(<Harness onEditor={() => {}} open={true} />)
    await waitFor(() => {
      expect(screen.getByText("Country")).toBeTruthy()
    })
    const filter = screen.getByTestId("ve-merge-tag-filter")
    fireEvent.change(filter, { target: { value: "unsub" } })
    expect(screen.queryByText("First name")).toBeNull()
    expect(screen.queryByText("Country")).toBeNull()
    expect(screen.getByText("Unsubscribe URL")).toBeTruthy()
  })

  it("inserts a mergeTag JSON node into the editor on selection", async () => {
    vi.mocked(api.mergeTags.list).mockResolvedValue(ok(sampleResponse))
    let editor: Editor | undefined
    renderWithClient(
      <Harness
        onEditor={(e) => {
          editor = e
        }}
        open={true}
      />,
    )
    await waitFor(() => {
      expect(editor).toBeDefined()
      expect(
        screen.getByTestId("ve-merge-tag-item-subscriber-first_name"),
      ).toBeTruthy()
    })
    fireEvent.click(
      screen.getByTestId("ve-merge-tag-item-subscriber-first_name"),
    )
    expect(editor).toBeDefined()
    const doc = editor!.getJSON() as {
      content?: Array<{ content?: Array<{ type: string; attrs?: object }> }>
    }
    const inline = doc.content?.[0]?.content?.[0]
    expect(inline?.type).toBe("mergeTag")
    expect(inline?.attrs).toMatchObject({
      namespace: "subscriber",
      key: "first_name",
    })
  })
})

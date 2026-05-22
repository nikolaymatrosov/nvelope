// MergeTagPicker — TanStack Query against `api.mergeTags.list`, grouped
// list of insertable placeholders. Opens in response to the
// `visual-editor:merge-tag-open` DOM event (dispatched by the slash menu
// or the bubble menu's "insert merge tag" action) and, on selection,
// inserts a `mergeTag` inline node into the editor.

import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { placeholderOf } from "../extensions/MergeTag"
import type { Editor } from "@tiptap/core"
import type {
  MergeTagCampaignItem,
  MergeTagPickerItem,
  MergeTagSubscriberItem,
  MergeTagsResponse,
} from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"

type Props = {
  slug: string
  editor: Editor | null
  // The picker is normally driven by the
  // `visual-editor:merge-tag-open` DOM event. The `open` /
  // `onOpenChange` props let test harnesses (and the bubble menu's
  // explicit button) control it directly without dispatching events.
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

export function MergeTagPicker({
  slug,
  editor,
  open: controlledOpen,
  onOpenChange,
}: Props) {
  const { t } = useTranslation("visualEditor")
  const [internalOpen, setInternalOpen] = useState(false)
  const open = controlledOpen ?? internalOpen
  const setOpen = (next: boolean) => {
    if (controlledOpen === undefined) setInternalOpen(next)
    onOpenChange?.(next)
  }

  const [filter, setFilter] = useState("")

  const query = useQuery({
    queryKey: queryKeys.mergeTags(slug),
    queryFn: async () => (await api.mergeTags.list(slug)).data,
    enabled: open,
  })

  useEffect(() => {
    if (!editor) return
    const onOpen = () => setOpen(true)
    const root = editor.view.dom
    root.addEventListener("visual-editor:merge-tag-open", onOpen)
    return () => {
      root.removeEventListener("visual-editor:merge-tag-open", onOpen)
    }
  }, [editor])

  const filtered = useMemo<{
    subscriber: Array<MergeTagSubscriberItem>
    campaign: Array<MergeTagCampaignItem>
  }>(() => {
    const data: MergeTagsResponse = query.data ?? {
      subscriber: [],
      campaign: [],
    }
    const f = filter.trim().toLowerCase()
    if (!f) return data
    const matches = (label: string, key: string) =>
      label.toLowerCase().includes(f) || key.toLowerCase().includes(f)
    return {
      subscriber: data.subscriber.filter((it) =>
        matches(it.displayName, it.slug),
      ),
      campaign: data.campaign.filter((it) =>
        matches(it.displayName, it.key),
      ),
    }
  }, [query.data, filter])

  if (!open) return null

  function insert(item: MergeTagPickerItem) {
    if (!editor) return
    const node =
      item.namespace === "subscriber"
        ? {
            type: "mergeTag",
            attrs: {
              namespace: "subscriber" as const,
              key: item.slug,
              label: item.displayName,
            },
          }
        : {
            type: "mergeTag",
            attrs: {
              namespace: "campaign" as const,
              key: item.key,
              label: item.displayName,
            },
          }
    editor.chain().focus().insertContent(node).run()
    setOpen(false)
    setFilter("")
  }

  return (
    <div
      role="dialog"
      aria-label={t("mergeTag.dialogAriaLabel")}
      data-testid="ve-merge-tag-picker"
      className="ve-merge-tag-picker"
      style={{
        position: "fixed",
        top: "20%",
        left: "50%",
        transform: "translateX(-50%)",
        zIndex: 60,
        background: "white",
        border: "1px solid #e5e7eb",
        borderRadius: 8,
        boxShadow: "0 8px 24px rgba(0,0,0,0.12)",
        padding: 12,
        width: 360,
        maxHeight: 480,
        overflowY: "auto",
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 8 }}>
        <strong style={{ fontSize: 14 }}>{t("mergeTag.title")}</strong>
        <button
          type="button"
          onClick={() => setOpen(false)}
          aria-label={t("mergeTag.closeAriaLabel")}
          style={{ background: "transparent", border: 0, cursor: "pointer" }}
        >
          ×
        </button>
      </div>
      <input
        autoFocus
        type="text"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        placeholder={t("mergeTag.filterPlaceholder")}
        data-testid="ve-merge-tag-filter"
        style={{
          width: "100%",
          padding: "6px 8px",
          border: "1px solid #e5e7eb",
          borderRadius: 4,
          marginBottom: 8,
        }}
      />
      {query.isLoading ? <p>{t("mergeTag.loading")}</p> : null}
      {query.isError ? (
        <p style={{ color: "#b91c1c" }}>{t("mergeTag.loadError")}</p>
      ) : null}

      {filtered.subscriber.length > 0 ? (
        <section>
          <h4 style={{ fontSize: 12, color: "#6b7280", margin: "8px 0 4px" }}>
            {t("mergeTag.groupSubscriber")}
          </h4>
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {filtered.subscriber.map((item) => (
              <li key={`s-${item.slug}`}>
                <button
                  type="button"
                  data-testid={`ve-merge-tag-item-subscriber-${item.slug}`}
                  onClick={() =>
                    insert({ namespace: "subscriber", ...item })
                  }
                  style={itemButtonStyle}
                >
                  <span>{item.displayName}</span>
                  <code style={codeStyle}>
                    {placeholderOf({ namespace: "subscriber", key: item.slug })}
                  </code>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {filtered.campaign.length > 0 ? (
        <section>
          <h4 style={{ fontSize: 12, color: "#6b7280", margin: "8px 0 4px" }}>
            {t("mergeTag.groupCampaign")}
          </h4>
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {filtered.campaign.map((item) => (
              <li key={`c-${item.key}`}>
                <button
                  type="button"
                  data-testid={`ve-merge-tag-item-campaign-${item.key}`}
                  onClick={() =>
                    insert({ namespace: "campaign", ...item })
                  }
                  style={itemButtonStyle}
                >
                  <span>{item.displayName}</span>
                  <code style={codeStyle}>
                    {placeholderOf({ namespace: "campaign", key: item.key })}
                  </code>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {!query.isLoading &&
      filtered.subscriber.length === 0 &&
      filtered.campaign.length === 0 ? (
        <p style={{ color: "#6b7280", fontSize: 12 }}>{t("mergeTag.noMatches")}</p>
      ) : null}
    </div>
  )
}

const itemButtonStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: 8,
  width: "100%",
  padding: "6px 8px",
  textAlign: "left",
  background: "transparent",
  border: 0,
  cursor: "pointer",
  borderRadius: 4,
}

const codeStyle: React.CSSProperties = {
  fontSize: 11,
  color: "#6b7280",
  background: "#f3f4f6",
  padding: "1px 4px",
  borderRadius: 3,
}

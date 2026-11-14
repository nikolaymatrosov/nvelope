// useVisualSave — the editing session's save state, shared by the template and
// campaign editors: the working document + theme, a dirty flag (current vs
// last-saved snapshot), and the visual save with the FR-009 optimistic-
// concurrency flow. On 409 stale_row a toast offers Reload (take the server
// copy) or Force overwrite (re-send the operator's copy with a fresh token).

import { useMemo, useRef, useState } from "react"
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
  // False for read-only sessions: they are never dirty.
  editable?: boolean
  save: VisualSaveFn
  refetch: VisualRefetchFn
  onSaved?: () => Promise<void>
}

// canon normalises what the stored doc (Postgres jsonb key order, null attrs)
// and the editor's getJSON() (TipTap key order, explicit null attrs) disagree on.
function canon(v: unknown, parentKey?: string): unknown {
  if (Array.isArray(v)) return v.map((x) => canon(x))
  if (v === null || typeof v !== "object") return v
  const out: Record<string, unknown> = {}
  for (const k of Object.keys(v).sort()) {
    const x = (v as Record<string, unknown>)[k]
    if (parentKey === "attrs" && x == null) continue
    const c = canon(x, k)
    if (k === "attrs" && (c === null || Object.keys(c as object).length === 0)) continue
    out[k] = c
  }
  return out
}

const isEmptyParagraph = (n: unknown): boolean => {
  const node = n as { type?: string; content?: Array<unknown>; attrs?: Record<string, unknown> }
  return (
    node.type === "paragraph" &&
    (node.content?.length ?? 0) === 0 &&
    Object.values(node.attrs ?? {}).every((x) => x == null)
  )
}

// keyOf: canonical snapshot key. Ignores ONE trailing empty top-level
// paragraph (TipTap's TrailingNode appends it after a non-paragraph block).
export function keyOf(s: VisualSnapshot): string {
  const content = [...s.bodyDoc.content]
  if (content.length > 1 && isEmptyParagraph(content[content.length - 1])) content.pop()
  return JSON.stringify(canon([{ ...s.bodyDoc, content }, s.theme]))
}

export function useVisualSave({ initial, initiallyDirty = false, editable = true, save, refetch, onSaved }: Options) {
  const { t } = useTranslation("visualEditor")
  const [bodyDoc, setBodyDoc] = useState(initial.bodyDoc)
  const [theme, setTheme] = useState(initial.theme)
  const [savedKey, setSavedKey] = useState<string | null>(() => (initiallyDirty ? null : keyOf(initial)))
  // The row's last-known updated_at (FR-009 token). A ref, not state, so a
  // retry issued from a toast callback always reads the latest value.
  const token = useRef(initial.updatedAt)
  const currentKey = useMemo(() => keyOf({ bodyDoc, theme }), [bodyDoc, theme])
  const dirty = editable && savedKey !== currentKey

  async function reload() {
    const fresh = await refetch()
    setBodyDoc(fresh.bodyDoc)
    setTheme(fresh.theme)
    token.current = fresh.updatedAt
    setSavedKey(keyOf(fresh))
  }

  // True from the moment a save is issued until it settles. A ref, not
  // mutation.isPending: two calls in one render see the same stale isPending.
  const inFlight = useRef(false)

  const mutation = useMutation({
    mutationFn: (v: { snapshot: VisualSnapshot; ifUnmodifiedSince: string }) =>
      save({ ...v.snapshot, ifUnmodifiedSince: v.ifUnmodifiedSince }),
    onSuccess: (res, v) => {
      token.current = res.data.updatedAt
      // Mark what was *sent* as saved: edits typed during the request stay dirty.
      setSavedKey(keyOf(v.snapshot))
      const warnings = res.data.warnings.length
      if (warnings > 0) toast.warning(t("save.warnings", { count: warnings }))
      else toast.success(t("save.success"))
      // Not awaited: a failing onSaved must not reach onError and report a persisted save as failed.
      onSaved?.().catch((err: unknown) => toast.error(errorMessage(err)))
    },
    onSettled: () => {
      inFlight.current = false
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
    if (inFlight.current) return
    inFlight.current = true
    try {
      const ifUnmodifiedSince = currentUpdatedAt ?? (await refetch()).updatedAt
      mutation.mutate({ snapshot, ifUnmodifiedSince })
    } catch (err) {
      inFlight.current = false
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
    save: () => {
      if (inFlight.current) return
      inFlight.current = true
      mutation.mutate({ snapshot: { bodyDoc, theme }, ifUnmodifiedSince: token.current })
    },
  }
}

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

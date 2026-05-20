// Branding (US4 part B). Lets a tenant administrator save the logo URL,
// primary color, and custom CSS applied to every one of the tenant's public
// pages. CSS is sanitized server-side; the editor shows the sanitized result
// returned on save as a read-only preview block.

import { createFileRoute } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { BrandingView, MediaAssetView } from "@/lib/api-types"
import { CUSTOM_CSS_LIMIT_BYTES } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { AsyncState } from "@/components/common/async-state"
import { CssEditor, isCssOverLimit } from "@/components/common/css-editor"
import { MediaPicker } from "@/components/common/media-picker"

export const Route = createFileRoute("/t/$slug/branding/")({
  component: BrandingView_,
})

const DEFAULT_COLOR = "#4f46e5"

export function BrandingView_() {
  const { slug } = Route.useParams()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)
  const canManage = can("branding:manage")

  const brandingQuery = useQuery({
    queryKey: queryKeys.branding(slug),
    queryFn: async () => (await api.branding.get(slug)).data,
    enabled: canManage,
  })

  if (!canManage) {
    return (
      <Empty data-testid="branding-forbidden" className="border">
        <EmptyHeader>
          <EmptyTitle>You do not have access</EmptyTitle>
          <EmptyDescription>
            You need the branding:manage permission to view this page.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Branding</h1>
        <p className="text-sm text-muted-foreground">
          Applies to every one of your tenant's public pages — subscription,
          preference, archive, and the standalone campaign pages.
        </p>
      </header>

      <AsyncState query={brandingQuery}>
        {(branding) => (
          <BrandingForm
            slug={slug}
            branding={branding}
            onSaved={async () => {
              await queryClient.invalidateQueries({
                queryKey: queryKeys.branding(slug),
              })
            }}
          />
        )}
      </AsyncState>
    </div>
  )
}

type FormState = {
  logo_url: string
  primary_color: string
  custom_css: string
}

function BrandingForm({
  slug,
  branding,
  onSaved,
}: {
  slug: string
  branding: BrandingView
  onSaved: () => Promise<void>
}) {
  const [form, setForm] = useState<FormState>({
    logo_url: branding.logo_url,
    primary_color: branding.primary_color || DEFAULT_COLOR,
    custom_css: branding.custom_css,
  })
  const [pickerOpen, setPickerOpen] = useState(false)
  const [savedSanitized, setSavedSanitized] = useState<string | null>(
    branding.custom_css || null,
  )

  useEffect(() => {
    setForm({
      logo_url: branding.logo_url,
      primary_color: branding.primary_color || DEFAULT_COLOR,
      custom_css: branding.custom_css,
    })
    setSavedSanitized(branding.custom_css || null)
  }, [branding.logo_url, branding.primary_color, branding.custom_css])

  const save = useMutation({
    mutationFn: async () => (await api.branding.save(slug, form)).data,
    onSuccess: async (saved) => {
      setSavedSanitized(saved.custom_css || null)
      toast.success("Branding saved.")
      await onSaved()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const overLimit = isCssOverLimit(form.custom_css, CUSTOM_CSS_LIMIT_BYTES)

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Logo and color</CardTitle>
          <CardDescription>
            A logo and primary color shown at the top of every public page.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="logo-url">Logo URL</Label>
            <div className="flex gap-2">
              <Input
                id="logo-url"
                value={form.logo_url}
                onChange={(e) =>
                  setForm((f) => ({ ...f, logo_url: e.target.value }))
                }
                placeholder="https://"
                data-testid="logo-url-input"
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => setPickerOpen(true)}
                data-testid="logo-from-media"
              >
                Pick from media
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Use an image from your Media library or any public URL.
            </p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="primary-color">Primary color</Label>
            <div className="flex items-center gap-2">
              <Input
                id="primary-color"
                type="color"
                value={form.primary_color || DEFAULT_COLOR}
                onChange={(e) =>
                  setForm((f) => ({ ...f, primary_color: e.target.value }))
                }
                className="h-10 w-16 cursor-pointer p-1"
                data-testid="primary-color-input"
              />
              <Input
                value={form.primary_color}
                onChange={(e) =>
                  setForm((f) => ({ ...f, primary_color: e.target.value }))
                }
                placeholder="#RRGGBB"
                pattern="^#[0-9A-Fa-f]{6}$"
                className="max-w-[10rem]"
                data-testid="primary-color-hex"
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Custom CSS</CardTitle>
          <CardDescription>
            Applied to every public page. Markup that could escape or run
            scripts is removed by the server.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <CssEditor
            value={form.custom_css}
            onChange={(value) => setForm((f) => ({ ...f, custom_css: value }))}
            limitBytes={CUSTOM_CSS_LIMIT_BYTES}
            sanitized={savedSanitized}
          />
        </CardContent>
      </Card>

      <div className="flex items-center justify-end">
        <Button
          onClick={() => save.mutate()}
          disabled={save.isPending || overLimit}
          data-testid="save-branding"
        >
          {save.isPending ? "Saving…" : "Save branding"}
        </Button>
      </div>

      <MediaPicker
        slug={slug}
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onPick={(asset: MediaAssetView) =>
          setForm((f) => ({ ...f, logo_url: asset.public_url }))
        }
      />
    </>
  )
}

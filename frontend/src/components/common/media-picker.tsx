// Modal that browses the tenant's media library and inserts a reference to
// the picked asset. Used from the campaign HTML-body field; reusable from any
// future authoring surface. Requires `media:get` to list — callers gate the
// opening button accordingly.

import { useQuery } from "@tanstack/react-query"
import { ImageOffIcon } from "lucide-react"
import type { MediaAssetView } from "@/lib/api-types"
import { isImageContentType } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"

type MediaPickerProps = {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onPick: (asset: MediaAssetView) => void
}

export function MediaPicker({ slug, open, onOpenChange, onPick }: MediaPickerProps) {
  const mediaQuery = useQuery({
    queryKey: queryKeys.media(slug),
    queryFn: async () => (await api.media.list(slug)).data,
    enabled: open,
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Insert from media library</DialogTitle>
          <DialogDescription>
            Pick an asset to insert into the message. Only your tenant's media is shown.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          <AsyncState
            query={mediaQuery}
            isEmpty={(d) => d.items.length === 0}
            emptyTitle="No media yet"
            emptyMessage="Upload files in the Media library before inserting them here."
            emptyAction={
              <a
                href={`/t/${slug}/media`}
                className="text-sm font-medium text-primary underline-offset-4 hover:underline"
                onClick={() => onOpenChange(false)}
              >
                Open Media library
              </a>
            }
          >
            {(data) => (
              <ul
                className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4"
                data-testid="media-picker-grid"
              >
                {data.items.map((asset) => (
                  <li key={asset.id}>
                    <button
                      type="button"
                      className="group flex w-full flex-col gap-1 rounded-md border bg-background p-2 text-left hover:border-primary focus:outline-none focus:ring-2 focus:ring-primary"
                      onClick={() => {
                        onPick(asset)
                        onOpenChange(false)
                      }}
                      data-testid={`media-picker-item-${asset.id}`}
                    >
                      <div className="grid aspect-square place-items-center overflow-hidden rounded bg-muted">
                        {isImageContentType(asset.content_type) ? (
                          <img
                            src={asset.public_url}
                            alt={asset.filename}
                            className="size-full object-contain"
                            loading="lazy"
                          />
                        ) : (
                          <ImageOffIcon className="size-8 text-muted-foreground" />
                        )}
                      </div>
                      <span className="truncate text-xs font-medium" title={asset.filename}>
                        {asset.filename}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {asset.content_type}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </AsyncState>
        </div>
      </DialogContent>
    </Dialog>
  )
}

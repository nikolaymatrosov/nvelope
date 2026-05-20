// Media library (US5). Browse, upload, copy, and delete tenant media. Uploads
// stream multipart to the backend; oversized and disallowed-type files are
// rejected up front, before any network call, so nothing partial reaches the
// listing. Cross-tenant access is impossible — every API call goes through
// the tenant-scoped client.

import { Link, createFileRoute } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CopyIcon, ExternalLinkIcon, ImageOffIcon, Trash2Icon, UploadIcon } from "lucide-react"
import { toast } from "sonner"
import type { MediaAssetView } from "@/lib/api-types"
import {
  ALLOWED_MEDIA_CONTENT_TYPES,
  DEFAULT_MEDIA_MAX_BYTES,
  isImageContentType,
} from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isForbidden } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
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
import { ConfirmDialog } from "@/components/common/confirm-dialog"

export const Route = createFileRoute("/t/$slug/media/")({
  component: MediaLibrary,
})

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function validateFile(file: File): string | null {
  if (file.size > DEFAULT_MEDIA_MAX_BYTES) {
    return `File is too large. The limit is ${humanSize(DEFAULT_MEDIA_MAX_BYTES)}.`
  }
  if (!ALLOWED_MEDIA_CONTENT_TYPES.includes(file.type)) {
    return `Files of type "${file.type || "unknown"}" are not allowed. Allowed: ${ALLOWED_MEDIA_CONTENT_TYPES.join(", ")}.`
  }
  return null
}

export function MediaLibrary() {
  const { slug } = Route.useParams()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)
  const canView = can("media:get") || can("media:manage")
  const canManage = can("media:manage")

  const mediaQuery = useQuery({
    queryKey: queryKeys.media(slug),
    queryFn: async () => (await api.media.list(slug)).data,
    enabled: canView,
  })

  if (!canView) {
    return (
      <Empty data-testid="media-forbidden" className="border">
        <EmptyHeader>
          <EmptyTitle>You do not have access</EmptyTitle>
          <EmptyDescription>
            You need the media:get or media:manage permission to view the
            library.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Media library</h1>
        <p className="text-sm text-muted-foreground">
          Upload images and documents to reference from your campaigns. Limit
          per file: {humanSize(DEFAULT_MEDIA_MAX_BYTES)}.
        </p>
      </header>

      {canManage && <UploadControl slug={slug} />}

      <AsyncState
        query={mediaQuery}
        isEmpty={(d) => d.items.length === 0}
        emptyTitle="No media yet"
        emptyMessage={
          canManage
            ? "Upload your first asset above."
            : "Nothing in the library yet."
        }
      >
        {(data) => (
          <ul
            className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4"
            data-testid="media-grid"
          >
            {data.items.map((asset) => (
              <MediaCard
                key={asset.id}
                slug={slug}
                asset={asset}
                canManage={canManage}
                onDeleted={() =>
                  queryClient.invalidateQueries({
                    queryKey: queryKeys.media(slug),
                  })
                }
              />
            ))}
          </ul>
        )}
      </AsyncState>
    </div>
  )
}

function UploadControl({ slug }: { slug: string }) {
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [inflight, setInflight] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)

  const upload = useMutation({
    mutationFn: (file: File) => api.media.upload(slug, file),
    onSuccess: async () => {
      toast.success("Uploaded.")
      await queryClient.invalidateQueries({ queryKey: queryKeys.media(slug) })
    },
    onError: (e) => {
      if (isForbidden(e)) {
        toast.error("You do not have permission to upload media.")
        return
      }
      toast.error(errorMessage(e))
    },
    onSettled: () => {
      setInflight(false)
      if (inputRef.current) inputRef.current.value = ""
    },
  })

  function handleFile(file: File) {
    const err = validateFile(file)
    if (err) {
      setValidationError(err)
      toast.error(err)
      if (inputRef.current) inputRef.current.value = ""
      return
    }
    setValidationError(null)
    setInflight(true)
    upload.mutate(file)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Upload</CardTitle>
        <CardDescription>
          Allowed types: {ALLOWED_MEDIA_CONTENT_TYPES.join(", ")}.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <input
          ref={inputRef}
          type="file"
          accept={ALLOWED_MEDIA_CONTENT_TYPES.join(",")}
          onChange={(e) => {
            const file = e.target.files?.[0]
            if (file) handleFile(file)
          }}
          className="hidden"
          data-testid="media-upload-input"
        />
        <Button
          onClick={() => inputRef.current?.click()}
          disabled={inflight}
          data-testid="media-upload-button"
          className="self-start"
        >
          <UploadIcon /> {inflight ? "Uploading…" : "Upload file"}
        </Button>
        {validationError && (
          <p
            className="text-sm text-destructive"
            role="alert"
            data-testid="media-upload-error"
          >
            {validationError}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function MediaCard({
  slug,
  asset,
  canManage,
  onDeleted,
}: {
  slug: string
  asset: MediaAssetView
  canManage: boolean
  onDeleted: () => void
}) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const remove = useMutation({
    mutationFn: () => api.media.remove(slug, asset.id),
    onSuccess: () => {
      toast.success("Deleted.")
      setConfirmOpen(false)
      onDeleted()
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      setConfirmOpen(false)
    },
  })

  function copyUrl() {
    navigator.clipboard.writeText(asset.public_url).then(
      () => toast.success("Copied"),
      () => toast.error("Could not copy"),
    )
  }

  return (
    <li
      className="flex flex-col gap-2 rounded-md border p-3"
      data-testid={`media-card-${asset.id}`}
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
      <div className="flex flex-col gap-0.5">
        <Link
          to="/t/$slug/media/$id"
          params={{ slug, id: asset.id }}
          className="truncate text-sm font-medium hover:underline"
          title={asset.filename}
        >
          {asset.filename}
        </Link>
        <p className="text-xs text-muted-foreground">
          {asset.content_type} · {humanSize(asset.size_bytes)} · {formatDate(asset.created_at)}
        </p>
      </div>
      <div className="flex items-center justify-between gap-1">
        <div className="flex gap-1">
          <Button
            variant="outline"
            size="sm"
            onClick={copyUrl}
            data-testid={`media-copy-${asset.id}`}
          >
            <CopyIcon /> Copy URL
          </Button>
          <Button variant="ghost" size="sm" asChild>
            <a href={asset.public_url} target="_blank" rel="noreferrer">
              <ExternalLinkIcon /> Open
            </a>
          </Button>
        </div>
        {canManage && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setConfirmOpen(true)}
            aria-label={`Delete ${asset.filename}`}
            data-testid={`media-delete-${asset.id}`}
          >
            <Trash2Icon />
          </Button>
        )}
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete this asset?"
        description="It will be removed from the library. Campaigns that reference its URL will lose the asset."
        confirmLabel="Delete"
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </li>
  )
}

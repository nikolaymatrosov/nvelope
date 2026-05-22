// Media asset detail (US5). Reuses the library-list query (the backend
// exposes no GET /media/{id}) to look up one asset by id, presenting its
// preview, metadata, and a prominent copy-URL control.

import { Link, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { CopyIcon, ImageOffIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import { isImageContentType } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
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

export const Route = createFileRoute("/t/$slug/media/$id")({
  component: MediaDetail,
})

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function MediaDetail() {
  const { slug, id } = Route.useParams()
  const { t } = useTranslation("media")
  const { can } = usePermissions(slug)
  const canView = can("media:get") || can("media:manage")

  const mediaQuery = useQuery({
    queryKey: queryKeys.media(slug),
    queryFn: async () => (await api.media.list(slug)).data,
    enabled: canView,
  })

  if (!canView) {
    return (
      <Empty data-testid="media-detail-forbidden" className="border">
        <EmptyHeader>
          <EmptyTitle>{t("forbidden.title")}</EmptyTitle>
          <EmptyDescription>
            {t("forbidden.assetDescription")}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/media"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          {t("library.backLink")}
        </Link>
      </div>
      <AsyncState query={mediaQuery}>
        {(data) => {
          const asset = data.items.find((a) => a.id === id)
          if (!asset) {
            return (
              <Empty data-testid="media-asset-not-found" className="border">
                <EmptyHeader>
                  <EmptyTitle>{t("detail.notFoundTitle")}</EmptyTitle>
                  <EmptyDescription>
                    {t("detail.notFoundDescription")}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )
          }
          const copyUrl = () =>
            navigator.clipboard.writeText(asset.public_url).then(
              () => toast.success(t("toast.copied")),
              () => toast.error(t("toast.copyFailed")),
            )
          return (
            <>
              <header>
                <h1
                  className="break-all text-2xl font-semibold"
                  data-testid="media-asset-filename"
                >
                  {asset.filename}
                </h1>
                <p className="text-sm text-muted-foreground">
                  {t("detail.uploadedMeta", {
                    type: asset.content_type,
                    size: humanSize(asset.size_bytes),
                    date: formatDate(asset.created_at),
                  })}
                </p>
              </header>
              <Card>
                <CardHeader>
                  <CardTitle>{t("detail.previewTitle")}</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="grid place-items-center overflow-hidden rounded bg-muted p-4">
                    {isImageContentType(asset.content_type) ? (
                      <img
                        src={asset.public_url}
                        alt={asset.filename}
                        className="max-h-[40rem] max-w-full object-contain"
                      />
                    ) : (
                      <ImageOffIcon className="size-12 text-muted-foreground" />
                    )}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>{t("detail.stableUrlTitle")}</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col gap-2">
                  <code
                    className="break-all rounded-md bg-muted p-2 text-sm"
                    data-testid="media-asset-url"
                  >
                    {asset.public_url}
                  </code>
                  <Button
                    onClick={copyUrl}
                    className="self-start"
                    data-testid="media-asset-copy"
                  >
                    <CopyIcon /> {t("card.copyUrl")}
                  </Button>
                </CardContent>
              </Card>
            </>
          )
        }}
      </AsyncState>
    </div>
  )
}

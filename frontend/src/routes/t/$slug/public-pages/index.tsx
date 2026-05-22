// Public pages — landing surface (US4). Lists the tenant's subscription pages,
// surfaces the per-tenant public URL bundle (subscription URLs, preference-link
// template, archive index, RSS feed), and is the entry point for create / edit.

import { Link, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"
import type { SubscriptionPageView } from "@/lib/api-types"
import type {PublicUrlRow} from "@/components/common/public-url-list";
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { AsyncState } from "@/components/common/async-state"
import {
  PublicUrlList

} from "@/components/common/public-url-list"

export const Route = createFileRoute("/t/$slug/public-pages/")({
  component: PublicPagesView,
})

function origin(): string {
  if (typeof window !== "undefined") {
    return window.location.origin
  }
  return ""
}

export function subscriptionPageUrl(slug: string, pageSlug: string): string {
  return `${origin()}/t/${slug}/subscribe/${pageSlug}`
}

export function archiveIndexUrl(slug: string): string {
  return `${origin()}/t/${slug}/archive`
}

export function rssFeedUrl(slug: string): string {
  return `${origin()}/t/${slug}/feed.xml`
}

export function preferenceTemplateUrl(): string {
  return `${origin()}/p/{token}`
}

export function buildPublicUrlRows(
  slug: string,
  pages: Array<SubscriptionPageView>,
  t: TFunction<"publicPages">,
): Array<PublicUrlRow> {
  const rows: Array<PublicUrlRow> = []
  for (const page of pages) {
    if (!page.Active) continue
    rows.push({
      kind: "subscription",
      label: t("publicUrls.subscriptionPage", { title: page.Title }),
      url: subscriptionPageUrl(slug, page.Slug),
    })
  }
  rows.push({
    kind: "preference-template",
    label: t("publicUrls.preferenceTemplate"),
    url: preferenceTemplateUrl(),
  })
  rows.push({
    kind: "archive",
    label: t("publicUrls.archiveIndex"),
    url: archiveIndexUrl(slug),
  })
  rows.push({
    kind: "rss",
    label: t("publicUrls.rssFeed"),
    url: rssFeedUrl(slug),
  })
  return rows
}

export function PublicPagesView() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("publicPages")
  const { can } = usePermissions(slug)
  const canManage = can("subscription_pages:manage")

  const pagesQuery = useQuery({
    queryKey: queryKeys.subscriptionPages(slug),
    queryFn: async () =>
      (await api.subscriptionPages.list(slug)).data.subscription_pages,
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("index.description")}
          </p>
        </div>
        {canManage && (
          <Button asChild>
            <Link
              to="/t/$slug/public-pages/$id"
              params={{ slug, id: "new" }}
              data-testid="new-subscription-page"
            >
              <PlusIcon /> {t("index.newPage")}
            </Link>
          </Button>
        )}
      </header>

      <AsyncState
        query={pagesQuery}
        isEmpty={(pages) => pages.length === 0}
        emptyTitle={t("index.emptyTitle")}
        emptyMessage={t("index.emptyMessage")}
        emptyAction={
          canManage ? (
            <Button asChild>
              <Link
                to="/t/$slug/public-pages/$id"
                params={{ slug, id: "new" }}
                data-testid="create-first-subscription-page"
              >
                <PlusIcon /> {t("index.createFirstPage")}
              </Link>
            </Button>
          ) : undefined
        }
      >
        {(pages) => (
          <>
            <Card>
              <CardHeader>
                <CardTitle>{t("index.subscriptionPagesTitle")}</CardTitle>
                <CardDescription>
                  {t("index.subscriptionPagesDescription")}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                {pages.map((page) => (
                  <SubscriptionPageRow key={page.ID} slug={slug} page={page} />
                ))}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>{t("index.shareUrlsTitle")}</CardTitle>
                <CardDescription>
                  {t("index.shareUrlsDescription")}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <PublicUrlList rows={buildPublicUrlRows(slug, pages, t)} />
              </CardContent>
            </Card>
          </>
        )}
      </AsyncState>
    </div>
  )
}

function SubscriptionPageRow({
  slug,
  page,
}: {
  slug: string
  page: SubscriptionPageView
}) {
  const { t } = useTranslation("publicPages")
  return (
    <div
      className="flex items-center justify-between gap-3 rounded-md border p-3"
      data-testid={`subscription-page-row-${page.ID}`}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate font-medium">{page.Title}</p>
          <Badge variant={page.Active ? "default" : "secondary"}>
            {page.Active ? t("row.active") : t("row.inactive")}
          </Badge>
        </div>
        <p
          className="truncate text-xs text-muted-foreground"
          title={subscriptionPageUrl(slug, page.Slug)}
        >
          {subscriptionPageUrl(slug, page.Slug)}
        </p>
      </div>
      <Button variant="outline" size="sm" asChild>
        <Link
          to="/t/$slug/public-pages/$id"
          params={{ slug, id: page.ID }}
          data-testid={`edit-subscription-page-${page.ID}`}
        >
          {t("row.edit")}
        </Link>
      </Button>
    </div>
  )
}

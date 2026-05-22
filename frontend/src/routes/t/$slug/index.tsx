import { Link, createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import {
  ArrowDownUpIcon,
  ListIcon,
  ScrollTextIcon,
  SettingsIcon,
  ShieldIcon,
  UsersIcon,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { useWorkspace } from "@/hooks/use-workspace"

export const Route = createFileRoute("/t/$slug/")({ component: Overview })

type Tile = {
  // Key under the `common` namespace's `overview.tiles` group.
  tileKey: "subscribers" | "lists" | "access" | "importExport" | "audit" | "settings"
  segment: string
  icon: LucideIcon
}

const TILES: Array<Tile> = [
  { tileKey: "subscribers", segment: "subscribers", icon: UsersIcon },
  { tileKey: "lists", segment: "lists", icon: ListIcon },
  { tileKey: "access", segment: "access", icon: ShieldIcon },
  { tileKey: "importExport", segment: "import-export", icon: ArrowDownUpIcon },
  { tileKey: "audit", segment: "audit", icon: ScrollTextIcon },
  { tileKey: "settings", segment: "settings", icon: SettingsIcon },
]

function Overview() {
  const { slug } = Route.useParams()
  const { name } = useWorkspace(slug)
  const { t } = useTranslation()

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold">
          {name ?? t("workspace.fallbackName")}
        </h1>
        <p className="text-sm text-muted-foreground">
          {t("overview.subtitle")}
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {TILES.map((tile) => {
          const to = `/t/${slug}/${tile.segment}`
          return (
          <Link key={tile.segment} to={to}>
            <Card className="h-full transition-colors hover:border-primary">
              <CardHeader>
                <tile.icon className="size-5 text-muted-foreground" />
                <CardTitle>
                  {t(`overview.tiles.${tile.tileKey}.label`)}
                </CardTitle>
                <CardDescription>
                  {t(`overview.tiles.${tile.tileKey}.description`)}
                </CardDescription>
              </CardHeader>
            </Card>
          </Link>
          )
        })}
      </div>
    </div>
  )
}

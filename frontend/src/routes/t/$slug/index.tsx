import { Link, createFileRoute } from "@tanstack/react-router"
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
  label: string
  segment: string
  description: string
  icon: LucideIcon
}

const TILES: Array<Tile> = [
  {
    label: "Subscribers",
    segment: "subscribers",
    description: "Browse, search, and manage subscribers.",
    icon: UsersIcon,
  },
  {
    label: "Lists",
    segment: "lists",
    description: "Organise subscribers into lists.",
    icon: ListIcon,
  },
  {
    label: "People & Access",
    segment: "access",
    description: "Invite teammates and manage roles.",
    icon: ShieldIcon,
  },
  {
    label: "Import / Export",
    segment: "import-export",
    description: "Bring subscribers in and out via CSV.",
    icon: ArrowDownUpIcon,
  },
  {
    label: "Audit",
    segment: "audit",
    description: "Review recent workspace activity.",
    icon: ScrollTextIcon,
  },
  {
    label: "Settings",
    segment: "settings",
    description: "Configure this workspace.",
    icon: SettingsIcon,
  },
]

function Overview() {
  const { slug } = Route.useParams()
  const { name } = useWorkspace(slug)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold">{name ?? "Workspace"}</h1>
        <p className="text-sm text-muted-foreground">
          Everything for this workspace, in one place.
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
                <CardTitle>{tile.label}</CardTitle>
                <CardDescription>{tile.description}</CardDescription>
              </CardHeader>
            </Card>
          </Link>
          )
        })}
      </div>
    </div>
  )
}

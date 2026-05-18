// Workspace navigation sidebar (FR-007, FR-008). Nav entries the user's
// permissions disallow are hidden; the active section is indicated by the
// router's current location.

import { Link, useLocation } from "@tanstack/react-router"
import {
  ArrowDownUpIcon,
  FileTextIcon,
  GlobeIcon,
  ListIcon,
  ScrollTextIcon,
  SendIcon,
  SettingsIcon,
  ShieldIcon,
  UsersIcon,
  ZapIcon,
} from "lucide-react"
import type { Permission } from "@/lib/api-types"
import type { LucideIcon } from "lucide-react"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { usePermissions } from "@/hooks/use-permissions"

type NavEntry = {
  label: string
  segment: string
  icon: LucideIcon
  // Visible when the user holds at least one of these. Empty = always visible.
  requires: Array<Permission>
}

const NAV: Array<NavEntry> = [
  { label: "Subscribers", segment: "subscribers", icon: UsersIcon, requires: ["subscribers:get"] },
  { label: "Lists", segment: "lists", icon: ListIcon, requires: ["lists:get"] },
  {
    label: "Sending Domains",
    segment: "sending-domains",
    icon: GlobeIcon,
    requires: ["sending:get", "sending:manage"],
  },
  {
    label: "Templates",
    segment: "templates",
    icon: FileTextIcon,
    requires: ["campaigns:get", "campaigns:manage"],
  },
  {
    label: "Campaigns",
    segment: "campaigns",
    icon: SendIcon,
    requires: ["campaigns:get", "campaigns:manage"],
  },
  {
    label: "Transactional Sending",
    segment: "transactional",
    icon: ZapIcon,
    requires: ["transactional:send", "apikeys:get", "apikeys:manage"],
  },
  { label: "People & Access", segment: "access", icon: ShieldIcon, requires: [] },
  {
    label: "Import / Export",
    segment: "import-export",
    icon: ArrowDownUpIcon,
    requires: ["subscribers:import", "subscribers:export"],
  },
  { label: "Audit", segment: "audit", icon: ScrollTextIcon, requires: ["audit:get"] },
  { label: "Settings", segment: "settings", icon: SettingsIcon, requires: ["settings:get"] },
]

export function WorkspaceSidebar({ slug }: { slug: string }) {
  const location = useLocation()
  const { canAny } = usePermissions(slug)
  const base = `/t/${slug}`

  return (
    <Sidebar>
      <SidebarHeader className="px-3 py-3">
        <Link to="/t/$slug" params={{ slug }} className="flex items-center gap-2">
          <span className="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground text-sm font-semibold">
            n
          </span>
          <span className="text-sm font-semibold">nvelope</span>
        </Link>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {NAV.filter(
                (entry) => entry.requires.length === 0 || canAny(entry.requires),
              ).map((entry) => {
                const href = `${base}/${entry.segment}`
                const active = location.pathname.startsWith(href)
                return (
                  <SidebarMenuItem key={entry.segment}>
                    <SidebarMenuButton asChild isActive={active}>
                      <Link to={href}>
                        <entry.icon />
                        <span>{entry.label}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}

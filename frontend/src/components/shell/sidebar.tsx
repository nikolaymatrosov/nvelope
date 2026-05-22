// Workspace navigation sidebar (FR-007, FR-008). Nav entries the user's
// permissions disallow are hidden; the active section is indicated by the
// router's current location.

import { Link, useLocation } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import {
  ArrowDownUpIcon,
  CreditCardIcon,
  FileTextIcon,
  GlobeIcon,
  ImageIcon,
  LayoutDashboardIcon,
  ListIcon,
  PaletteIcon,
  ScrollTextIcon,
  SendIcon,
  SettingsIcon,
  Share2Icon,
  ShieldIcon,
  ShieldXIcon,
  UsersIcon,
  ZapIcon,
} from "lucide-react"
import type { Permission } from "@/lib/api-types"
import type { LucideIcon } from "lucide-react"
import type Resources from "@/i18n/resources"
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

// A key under the `common` namespace's `nav` group.
type NavLabelKey = `nav.${keyof Resources["common"]["nav"]}`

type NavEntry = {
  labelKey: NavLabelKey
  segment: string
  icon: LucideIcon
  // Visible when the user holds at least one of these. Empty = always visible.
  requires: Array<Permission>
}

const NAV: Array<NavEntry> = [
  {
    labelKey: "nav.dashboard",
    segment: "dashboard",
    icon: LayoutDashboardIcon,
    requires: ["campaigns:get", "campaigns:manage"],
  },
  { labelKey: "nav.subscribers", segment: "subscribers", icon: UsersIcon, requires: ["subscribers:get"] },
  { labelKey: "nav.lists", segment: "lists", icon: ListIcon, requires: ["lists:get"] },
  {
    labelKey: "nav.sendingDomains",
    segment: "sending-domains",
    icon: GlobeIcon,
    requires: ["sending:get", "sending:manage"],
  },
  {
    labelKey: "nav.templates",
    segment: "templates",
    icon: FileTextIcon,
    requires: ["campaigns:get", "campaigns:manage"],
  },
  {
    labelKey: "nav.campaigns",
    segment: "campaigns",
    icon: SendIcon,
    requires: ["campaigns:get", "campaigns:manage"],
  },
  {
    labelKey: "nav.suppressions",
    segment: "suppressions",
    icon: ShieldXIcon,
    requires: ["sending:get", "sending:manage"],
  },
  {
    labelKey: "nav.transactional",
    segment: "transactional",
    icon: ZapIcon,
    requires: ["transactional:send", "apikeys:get", "apikeys:manage"],
  },
  { labelKey: "nav.access", segment: "access", icon: ShieldIcon, requires: [] },
  {
    labelKey: "nav.importExport",
    segment: "import-export",
    icon: ArrowDownUpIcon,
    requires: ["subscribers:import", "subscribers:export"],
  },
  { labelKey: "nav.audit", segment: "audit", icon: ScrollTextIcon, requires: ["audit:get"] },
  {
    labelKey: "nav.billing",
    segment: "billing",
    icon: CreditCardIcon,
    requires: ["billing:get", "billing:manage"],
  },
  {
    labelKey: "nav.publicPages",
    segment: "public-pages",
    icon: Share2Icon,
    requires: ["subscription_pages:manage"],
  },
  {
    labelKey: "nav.branding",
    segment: "branding",
    icon: PaletteIcon,
    requires: ["branding:manage"],
  },
  {
    labelKey: "nav.media",
    segment: "media",
    icon: ImageIcon,
    requires: ["media:get", "media:manage"],
  },
  { labelKey: "nav.settings", segment: "settings", icon: SettingsIcon, requires: ["settings:get"] },
]

export function WorkspaceSidebar({ slug }: { slug: string }) {
  const location = useLocation()
  const { canAny } = usePermissions(slug)
  const { t } = useTranslation("common")
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
          <SidebarGroupLabel>{t("nav.group")}</SidebarGroupLabel>
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
                        <span>{t(entry.labelKey)}</span>
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

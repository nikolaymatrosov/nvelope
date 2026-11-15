// The persistent workspace shell (FR-007, FR-009): sidebar + top bar wrapping
// every workspace screen. Permission-aware nav gating is handled inside the
// sidebar (entries the user cannot use are hidden).

import { useMatches } from "@tanstack/react-router"
import { WorkspaceSidebar } from "./sidebar"
import { TopBar } from "./top-bar"
import { SuspensionBanner } from "./suspension-banner"
import type { ReactNode } from "react"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { useSyncAccountLocale } from "@/hooks/use-locale"
import { usePlatformConfig } from "@/hooks/use-platform-config"

// Routes opt out of the workspace chrome with `staticData: { fullscreen: true }`
// (the visual email editor). Session/TOTP gating in /t/$slug still applies.
declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    fullscreen?: boolean
  }
}

type AppShellProps = {
  slug: string
  workspaceName: string
  children: ReactNode
}

export function AppShell({ slug, workspaceName, children }: AppShellProps) {
  // Apply the signed-in user's stored language preference (FR-004).
  useSyncAccountLocale()
  const { billing } = usePlatformConfig()
  const fullscreen = useMatches({
    select: (matches) => matches.some((m) => m.staticData.fullscreen === true),
  })

  if (fullscreen) return <>{children}</>

  return (
    <SidebarProvider>
      <WorkspaceSidebar slug={slug} />
      <SidebarInset>
        <TopBar workspaceName={workspaceName} />
        <main className="flex flex-1 flex-col gap-4 overflow-auto p-6">
          {billing && <SuspensionBanner slug={slug} />}
          {children}
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

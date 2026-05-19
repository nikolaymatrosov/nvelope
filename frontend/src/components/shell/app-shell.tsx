// The persistent workspace shell (FR-007, FR-009): sidebar + top bar wrapping
// every workspace screen. Permission-aware nav gating is handled inside the
// sidebar (entries the user cannot use are hidden).

import { WorkspaceSidebar } from "./sidebar"
import { TopBar } from "./top-bar"
import { SuspensionBanner } from "./suspension-banner"
import type { ReactNode } from "react"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"

type AppShellProps = {
  slug: string
  workspaceName: string
  children: ReactNode
}

export function AppShell({ slug, workspaceName, children }: AppShellProps) {
  return (
    <SidebarProvider>
      <WorkspaceSidebar slug={slug} />
      <SidebarInset>
        <TopBar workspaceName={workspaceName} />
        <main className="flex flex-1 flex-col gap-4 overflow-auto p-6">
          <SuspensionBanner slug={slug} />
          {children}
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

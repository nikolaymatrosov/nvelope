// Workspace top bar (FR-007): current workspace name plus the account /
// sign-out control.

import { useNavigate } from "@tanstack/react-router"
import { useMutation } from "@tanstack/react-query"
import { LogOutIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryClient } from "@/lib/query"
import { useSession } from "@/hooks/use-session"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { Separator } from "@/components/ui/separator"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

function initials(name: string): string {
  return (
    name
      .trim()
      .split(/\s+/)
      .map((p) => p.charAt(0).toUpperCase())
      .slice(0, 2)
      .join("") || "?"
  )
}

export function TopBar({ workspaceName }: { workspaceName: string }) {
  const navigate = useNavigate()
  const { user } = useSession()

  const logout = useMutation({
    mutationFn: () => api.logout(),
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: "/login" })
    },
  })

  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
      <SidebarTrigger />
      <Separator orientation="vertical" className="h-5" />
      <span className="text-sm font-semibold">{workspaceName}</span>
      <div className="ml-auto">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              className="flex items-center gap-2 rounded-md p-1 hover:bg-muted"
              aria-label="Account menu"
            >
              <Avatar className="size-7">
                <AvatarFallback>{initials(user?.name ?? "")}</AvatarFallback>
              </Avatar>
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>
              <div className="flex flex-col">
                <span>{user?.name}</span>
                <span className="text-xs font-normal text-muted-foreground">
                  {user?.email}
                </span>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => navigate({ to: "/" })}>
              Switch workspace
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={logout.isPending}
              onClick={() => logout.mutate()}
            >
              <LogOutIcon />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  )
}

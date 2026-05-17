// The signed-in user's derived workspace-level permissions (research.md
// Decision 3). Joins the platform account, the workspace member list, and the
// role catalogue. Roles may be unreadable without `roles:get`; in that case
// only the Owner shortcut resolves and other users fall back to an empty set
// (reactive 403 handling still applies).

import { useQuery } from "@tanstack/react-query"
import { useSession } from "./use-session"
import { useWorkspace } from "./use-workspace"
import type { EffectivePermissions } from "@/lib/permissions"
import type { Permission } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { can, canAny, derivePermissions } from "@/lib/permissions"

export function usePermissions(slug: string) {
  const { user, isLoading: sessionLoading } = useSession()
  const { members, isLoading: workspaceLoading } = useWorkspace(slug)

  const rolesQuery = useQuery({
    queryKey: queryKeys.roles(slug),
    queryFn: async () => (await api.listRoles(slug)).data.roles,
    retry: false,
  })

  const effective: EffectivePermissions = derivePermissions(
    user,
    members,
    rolesQuery.data ?? [],
  )

  return {
    effective,
    isLoading: sessionLoading || workspaceLoading || rolesQuery.isLoading,
    can: (permission: Permission) => can(effective, permission),
    canAny: (permissions: Array<Permission>) => canAny(effective, permissions),
  }
}

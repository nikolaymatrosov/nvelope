// Derive the signed-in user's effective workspace-level permissions
// (research.md Decision 3). No endpoint returns the resolved principal's
// permission set, so it is reconstructed by joining: my member entry (matched
// by email) → my role name → that role's Permissions.
//
// The backend stays authoritative — this only drives advisory nav/action
// gating (FR-009). Per-list grants are not enumerable; per-list gating is
// reactive on a 403.

import { ALL_PERMISSIONS } from "./api-types"
import type {
  Member,
  Permission,
  PlatformUser,
  Role,
} from "./api-types"

export type EffectivePermissions = {
  permissions: ReadonlySet<Permission>
  role: string | null
  isOwner: boolean
}

const OWNER_ROLE = "Owner"

export function derivePermissions(
  user: PlatformUser | null | undefined,
  members: Array<Member>,
  roles: Array<Role>,
): EffectivePermissions {
  if (!user) {
    return { permissions: new Set(), role: null, isOwner: false }
  }
  const mine = members.find(
    (m) => m.email.toLowerCase() === user.email.toLowerCase(),
  )
  if (!mine) {
    return { permissions: new Set(), role: null, isOwner: false }
  }
  const isOwner = mine.role === OWNER_ROLE
  if (isOwner) {
    return {
      permissions: new Set(ALL_PERMISSIONS),
      role: mine.role,
      isOwner: true,
    }
  }
  const role = roles.find((r) => r.Name === mine.role)
  const permissions = new Set<Permission>(role?.Permissions ?? [])
  return { permissions, role: mine.role, isOwner: false }
}

// `can` — gate a single action. Accepts the derived set directly.
export function can(
  effective: EffectivePermissions,
  permission: Permission,
): boolean {
  return effective.isOwner || effective.permissions.has(permission)
}

// `canAny` — true when the user holds at least one of the permissions (used to
// decide whether a whole nav section is reachable).
export function canAny(
  effective: EffectivePermissions,
  permissions: Array<Permission>,
): boolean {
  return effective.isOwner || permissions.some((p) => effective.permissions.has(p))
}

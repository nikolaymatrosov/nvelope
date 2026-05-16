# Contract — Permission Catalogue & RBAC Evaluation

The fixed set of permission strings for Phase 2 and the rules for evaluating a
principal's effective permissions. Permissions are flat `resource:action`
strings (research.md Decision 5).

## Catalogue

| Permission | Grants |
|---|---|
| `lists:get` | View lists. |
| `lists:manage` | Create, edit, delete lists; manage list membership. |
| `subscribers:get` | View and search subscribers; run segment queries. |
| `subscribers:manage` | Create, edit, delete subscribers; change state and memberships. |
| `subscribers:import` | Start imports; view import job status. |
| `subscribers:export` | Start exports; download export files. |
| `roles:get` | View roles. |
| `roles:manage` | Create, edit, delete roles; assign tenant-level and per-list roles. |
| `apikeys:get` | View API key metadata. |
| `apikeys:manage` | Issue and revoke API keys. |
| `audit:get` | View the audit log. |
| `settings:get` | View tenant settings. |
| `settings:manage` | Update tenant settings. |

A permission **string** must be in this catalogue; a role or API key referencing
an unknown string is rejected at construction (422).

`roles:manage` and `apikeys:manage` are administrative permissions — only users
holding them may manage roles or keys (FR-027). There is no implicit superuser;
a bootstrap **Owner** role carrying every permission is created with each tenant
and assigned to the tenant's first user.

## Per-list scope

`lists:get`, `lists:manage`, `subscribers:get`, `subscribers:manage` are the
permissions that can also be granted through a **per-list role**. The remaining
permissions are tenant-wide only — a per-list role carrying them has no
additional effect for a single list.

## Effective-permission evaluation

For an action that does **not** target a specific list:

```
allowed = principal.tenantPermissions.contains(required)
```

For an action targeting list `L`:

```
allowed = principal.tenantPermissions.contains(required)
       OR principal.listPermissions(L).contains(required)
```

`principal.listPermissions(L)` is the permission set of the user's per-list role
for `L`, or empty if none. The result is a **union** — a per-list role only ever
widens access for that list (research.md Decision 5; spec assumption).

A role's permission change takes effect on the holder's next request, because
the `Principal` is resolved fresh per request from current role data (FR-026).

## API key principals

An API key is itself a principal: its `tenantPermissions` are the key's scoped
subset and it has no per-list roles. A key therefore cannot exceed its scope
(FR-031), and a revoked key resolves to no principal at all (401).

## Denial

A failed check produces `apperr` with category `Forbidden` and a slug naming the
missing permission (e.g. `forbidden-lists-manage`). No state is changed. `errmap`
maps `Forbidden → 403`. An unauthenticated request (no/invalid session or key)
produces `Authorization → 401` instead.

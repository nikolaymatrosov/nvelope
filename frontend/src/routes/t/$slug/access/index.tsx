import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CopyIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import type { Member, Permission, Role } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { ALL_PERMISSIONS } from "@/lib/api-types"
import { formatDate } from "@/lib/format"
import { useWorkspace } from "@/hooks/use-workspace"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/access/")({
  component: AccessView,
})

export function AccessView() {
  const { slug } = Route.useParams()
  const { can } = usePermissions(slug)
  const canManageRoles = can("roles:manage")

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">People & Access</h1>
        <p className="text-sm text-muted-foreground">
          Manage members, invitations, and roles.
        </p>
      </header>

      <Tabs defaultValue="members">
        <TabsList>
          <TabsTrigger value="members">Members</TabsTrigger>
          <TabsTrigger value="invitations">Invitations</TabsTrigger>
          <TabsTrigger value="roles">Roles</TabsTrigger>
        </TabsList>
        <TabsContent value="members" className="pt-4">
          <MembersPanel slug={slug} canManageRoles={canManageRoles} />
        </TabsContent>
        <TabsContent value="invitations" className="pt-4">
          <InvitationsPanel slug={slug} />
        </TabsContent>
        <TabsContent value="roles" className="pt-4">
          <RolesPanel slug={slug} canManageRoles={canManageRoles} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ── Members ──────────────────────────────────────────────────────────────────

export function MembersPanel({
  slug,
  canManageRoles,
}: {
  slug: string
  canManageRoles: boolean
}) {
  const { members, isLoading, isError, error } = useWorkspace(slug)
  const rolesQuery = useQuery({
    queryKey: queryKeys.roles(slug),
    queryFn: async () => (await api.listRoles(slug)).data.roles,
  })

  if (isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Could not load members</AlertTitle>
        <AlertDescription>{errorMessage(error)}</AlertDescription>
      </Alert>
    )
  }
  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading members…</p>
  }

  return (
    <div className="flex flex-col gap-3">
      {!canManageRoles && (
        <Alert>
          <AlertTitle>View only</AlertTitle>
          <AlertDescription>
            You can see members but need the “roles:manage” permission to
            change role assignments.
          </AlertDescription>
        </Alert>
      )}
      {members.map((member) => (
        <MemberRow
          key={member.user_id}
          slug={slug}
          member={member}
          roles={rolesQuery.data ?? []}
          canManageRoles={canManageRoles}
        />
      ))}
    </div>
  )
}

function MemberRow({
  slug,
  member,
  roles,
  canManageRoles,
}: {
  slug: string
  member: Member
  roles: Array<Role>
  canManageRoles: boolean
}) {
  const queryClient = useQueryClient()
  const [listRolesOpen, setListRolesOpen] = useState(false)
  const currentRole = roles.find((r) => r.Name === member.role)

  const assign = useMutation({
    mutationFn: (roleId: string) => api.assignRole(slug, member.user_id, roleId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.tenant(slug) })
      toast.success("Role updated.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex items-center gap-3 rounded-lg border p-3">
      <div className="flex-1">
        <p className="text-sm font-medium">{member.name}</p>
        <p className="text-xs text-muted-foreground">{member.email}</p>
      </div>
      {canManageRoles && roles.length > 0 ? (
        <Select
          value={currentRole?.ID ?? ""}
          onValueChange={(roleId) => assign.mutate(roleId)}
        >
          <SelectTrigger className="w-44">
            <SelectValue placeholder={member.role} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {roles.map((r) => (
                <SelectItem key={r.ID} value={r.ID}>
                  {r.Name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      ) : (
        <Badge variant="secondary">{member.role}</Badge>
      )}
      {canManageRoles && (
        <Button
          variant="outline"
          size="sm"
          onClick={() => setListRolesOpen(true)}
        >
          List access
        </Button>
      )}
      <ListRolesDialog
        slug={slug}
        member={member}
        roles={roles}
        open={listRolesOpen}
        onOpenChange={setListRolesOpen}
      />
    </div>
  )
}

function ListRolesDialog({
  slug,
  member,
  roles,
  open,
  onOpenChange,
}: {
  slug: string
  member: Member
  roles: Array<Role>
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [listId, setListId] = useState("")
  const [roleId, setRoleId] = useState("")

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 100, offset: 0 })).data.lists,
    enabled: open,
  })

  const grant = useMutation({
    mutationFn: () => api.assignListRole(slug, member.user_id, listId, roleId),
    onSuccess: () => {
      toast.success("Per-list role granted.")
      setListId("")
      setRoleId("")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const revoke = useMutation({
    mutationFn: () => api.removeListRole(slug, member.user_id, listId),
    onSuccess: () => {
      toast.success("Per-list role removed.")
      setListId("")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>List access for {member.name}</DialogTitle>
          <DialogDescription>
            Grant or remove a role scoped to a single list.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <FormField label="List">
            <Select value={listId} onValueChange={setListId}>
              <SelectTrigger>
                <SelectValue placeholder="Choose a list…" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {(listsQuery.data ?? []).map((l) => (
                    <SelectItem key={l.ID} value={l.ID}>
                      {l.Name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label="Role">
            <Select value={roleId} onValueChange={setRoleId}>
              <SelectTrigger>
                <SelectValue placeholder="Choose a role…" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {roles.map((r) => (
                    <SelectItem key={r.ID} value={r.ID}>
                      {r.Name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            disabled={!listId || revoke.isPending}
            onClick={() => revoke.mutate()}
          >
            Remove role on list
          </Button>
          <Button
            disabled={!listId || !roleId || grant.isPending}
            onClick={() => grant.mutate()}
          >
            Grant role
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ── Invitations ──────────────────────────────────────────────────────────────

export function InvitationsPanel({ slug }: { slug: string }) {
  const queryClient = useQueryClient()
  const [acceptUrl, setAcceptUrl] = useState("")
  const [revoking, setRevoking] = useState<string | null>(null)

  const invitationsQuery = useQuery({
    queryKey: queryKeys.invitations(slug),
    queryFn: async () => (await api.listInvitations(slug)).data.invitations,
  })

  const invite = useMutation({
    mutationFn: (email: string) => api.invite(slug, email.trim()),
    onSuccess: async (res) => {
      setAcceptUrl(res.data.accept_url)
      await queryClient.invalidateQueries({
        queryKey: queryKeys.invitations(slug),
      })
      toast.success("Invitation sent.")
      form.reset()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeInvitation(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.invitations(slug),
      })
      toast.success("Invitation revoked.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { email: "" },
    onSubmit: async ({ value }) => {
      await invite.mutateAsync(value.email).catch(() => {})
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Invite a teammate</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex items-end gap-2"
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              form.handleSubmit()
            }}
          >
            <div className="flex-1">
              <form.Field
                name="email"
                validators={{ onBlur: compose(rules.required(), rules.email()) }}
              >
                {(field) => (
                  <FormField
                    label="Email address"
                    type="email"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    error={fieldError(field.state.meta.errors)}
                  />
                )}
              </form.Field>
            </div>
            <Button type="submit" disabled={invite.isPending}>
              Send invite
            </Button>
          </form>
          {acceptUrl && (
            <Alert className="mt-3">
              <AlertTitle>Invitation link</AlertTitle>
              <AlertDescription>
                <span className="flex items-center gap-2">
                  <code className="truncate text-xs">{acceptUrl}</code>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Copy link"
                    onClick={() => {
                      navigator.clipboard.writeText(acceptUrl)
                      toast.success("Link copied.")
                    }}
                  >
                    <CopyIcon />
                  </Button>
                </span>
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      <AsyncState
        query={invitationsQuery}
        isEmpty={(d) => d.length === 0}
        emptyTitle="No pending invitations"
        emptyMessage="Invitations you send will appear here until accepted."
      >
        {(invitations) => (
          <div className="flex flex-col gap-2">
            {invitations.map((inv) => (
              <div
                key={inv.id}
                className="flex items-center gap-3 rounded-lg border p-3"
              >
                <div className="flex-1">
                  <p className="text-sm font-medium">{inv.email}</p>
                  <p className="text-xs text-muted-foreground">
                    Expires {formatDate(inv.expires_at)}
                  </p>
                </div>
                <Badge variant="secondary">{inv.status}</Badge>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={revoke.isPending}
                  onClick={() => setRevoking(inv.id)}
                >
                  Revoke
                </Button>
              </div>
            ))}
          </div>
        )}
      </AsyncState>

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title="Revoke this invitation?"
        description="The invitation link will stop working and cannot be reused."
        confirmLabel="Revoke invitation"
        busy={revoke.isPending}
        onConfirm={() => {
          if (revoking) revoke.mutate(revoking)
          setRevoking(null)
        }}
      />
    </div>
  )
}

// ── Roles ────────────────────────────────────────────────────────────────────

export function RolesPanel({
  slug,
  canManageRoles,
}: {
  slug: string
  canManageRoles: boolean
}) {
  const [editing, setEditing] = useState<Role | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<Role | null>(null)
  const queryClient = useQueryClient()

  const rolesQuery = useQuery({
    queryKey: queryKeys.roles(slug),
    queryFn: async () => (await api.listRoles(slug)).data.roles,
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteRole(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.roles(slug) })
      toast.success("Role deleted.")
      setDeleting(null)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex flex-col gap-4">
      {!canManageRoles ? (
        <Alert>
          <AlertTitle>Role management unavailable</AlertTitle>
          <AlertDescription>
            You need the “roles:manage” permission to create or change roles.
          </AlertDescription>
        </Alert>
      ) : (
        <div>
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> New role
          </Button>
        </div>
      )}

      <AsyncState
        query={rolesQuery}
        isEmpty={(d) => d.length === 0}
        emptyTitle="No roles yet"
        emptyMessage="Create a role to grant scoped permissions."
      >
        {(roles) => (
          <div className="flex flex-col gap-2">
            {roles.map((role) => (
              <div
                key={role.ID}
                className="flex items-center gap-3 rounded-lg border p-3"
              >
                <div className="flex-1">
                  <p className="text-sm font-medium">{role.Name}</p>
                  <p className="text-xs text-muted-foreground">
                    {role.Permissions.length} permission
                    {role.Permissions.length === 1 ? "" : "s"}
                  </p>
                </div>
                {canManageRoles && (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setEditing(role)}
                    >
                      Edit
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Delete role"
                      onClick={() => setDeleting(role)}
                    >
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </div>
            ))}
          </div>
        )}
      </AsyncState>

      {(creating || editing) && (
        <RoleDialog
          slug={slug}
          role={editing}
          open
          onClose={() => {
            setCreating(false)
            setEditing(null)
          }}
        />
      )}

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete the “${deleting?.Name}” role?`}
        description="Members assigned this role will lose its permissions."
        confirmLabel="Delete role"
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.ID)}
      />
    </div>
  )
}

function RoleDialog({
  slug,
  role,
  open,
  onClose,
}: {
  slug: string
  role: Role | null
  open: boolean
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [permissions, setPermissions] = useState<Array<Permission>>(
    role?.Permissions ?? [],
  )

  const save = useMutation({
    mutationFn: (name: string) =>
      role
        ? api.updateRole(slug, role.ID, { name: name.trim(), permissions })
        : api.createRole(slug, name.trim(), permissions),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.roles(slug) })
      toast.success(role ? "Role updated." : "Role created.")
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: role?.Name ?? "" },
    onSubmit: async ({ value }) => {
      await save.mutateAsync(value.name).catch(() => {})
    },
  })

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{role ? "Edit role" : "New role"}</DialogTitle>
          <DialogDescription>
            Name the role and choose the permissions it grants.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            form.handleSubmit()
          }}
        >
          <form.Field
            name="name"
            validators={{ onBlur: compose(rules.required("Enter a role name.")) }}
          >
            {(field) => (
              <FormField
                label="Role name"
                required
                autoFocus
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <div className="flex flex-col gap-2">
            <p className="text-sm font-medium">Permissions</p>
            <div className="grid grid-cols-2 gap-2 rounded-lg border p-3">
              {ALL_PERMISSIONS.map((perm) => (
                <label key={perm} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={permissions.includes(perm)}
                    onCheckedChange={(checked) =>
                      setPermissions((prev) =>
                        checked
                          ? [...prev, perm]
                          : prev.filter((p) => p !== perm),
                      )
                    }
                  />
                  <code className="text-xs">{perm}</code>
                </label>
              ))}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "Saving…" : role ? "Save role" : "Create role"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

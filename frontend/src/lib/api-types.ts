// Frontend view model — the TypeScript types the UI consumes, mirrored from the
// backend's query read models and command inputs.
//
// Casing convention (research.md Decision 2): platform-plane and tenant
// settings/invitation responses are snake_case; audience and IAM responses are
// PascalCase (those Go view structs carry no `json:` tags). Field names below
// match exactly what each endpoint emits — do not rename them.

// ── Platform plane (snake_case) ──────────────────────────────────────────────

export type Membership = {
  id: string
  slug: string
  name: string
  status: string
  role: string
}

export type PlatformUser = {
  id: string
  name: string
  email: string
}

export type PlatformAccount = {
  user: PlatformUser
  tenants: Array<Membership>
}

export type InvitationLookup = {
  email: string
  status: string
  expires_at: string
  tenant: { name: string; slug: string }
}

// ── Workspace / tenant plane (snake_case) ────────────────────────────────────

export type Member = {
  user_id: string
  email: string
  name: string
  role: string
}

export type WorkspaceInfo = {
  tenant: { name: string }
  members: Array<Member>
}

export type WorkspaceInvitation = {
  id: string
  email: string
  status: string
  created_at: string
  expires_at: string
}

export type WorkspaceSettings = {
  display_name: string
  timezone: string
}

// ── Workspace session state machine ──────────────────────────────────────────

export type SessionState = "active" | "totp_pending"

export type SessionResult = { state: SessionState }

// ── Audience plane — Lists & Subscribers (PascalCase) ────────────────────────

export type List = {
  ID: string
  Name: string
  Description: string
  Visibility: string
  OptIn: string
  Tags: Array<string>
  CreatedAt: string
  UpdatedAt: string
}

export type CreateListInput = {
  name: string
  description: string
  visibility?: string
  optin?: string
  tags?: Array<string>
}

export type UpdateListInput = {
  name: string
  description: string
  visibility?: string
  optin?: string
  tags?: Array<string>
}

export type SubscriberMembership = {
  ListID: string
  Status: string
}

export type Subscriber = {
  ID: string
  Email: string
  Name: string
  State: string
  Attributes: Record<string, unknown>
  Memberships: Array<SubscriberMembership>
  CreatedAt: string
  UpdatedAt: string
}

export type CreateSubscriberInput = {
  email: string
  name: string
  attributes: Record<string, unknown>
  list_ids: Array<string>
}

export type UpdateSubscriberInput = {
  name: string
  attributes: Record<string, unknown>
  state: string
}

// ── Segment query (PascalCase keys — the domain struct has no json tags) ─────

export type Conjunction = "and" | "or"

export type SegmentOp =
  | "eq"
  | "neq"
  | "exists"
  | "contains"
  | "gt"
  | "lt"
  | "gte"
  | "lte"

export type FieldName = "email" | "name" | "state"

export type FieldCondition = {
  Field: FieldName
  Op: SegmentOp
  Value: string
}

export type AttrCondition = {
  Key: string
  Op: SegmentOp
  Value: unknown
}

export type MemberCondition = {
  ListID: string
  Status: string
}

export type Node = {
  Conj?: Conjunction
  Children?: Array<Node>
  Field?: FieldCondition
  Attr?: AttrCondition
  Member?: MemberCondition
}

// ── Jobs — Import & Export (PascalCase) ──────────────────────────────────────

export type RowFailure = {
  Row: number
  Reason: string
}

export type JobStatusView = {
  ID: string
  Kind: string
  Status: string
  FileName: string
  CreatedCount: number
  UpdatedCount: number
  FailedCount: number
  RowCount: number
  Failures: Array<RowFailure>
}

export type ExportSelection = "all" | "list" | "segment"

export type StartExportInput = {
  selection: ExportSelection
  list_id?: string
  segment?: Node
}

export const TERMINAL_JOB_STATUSES = ["completed", "failed", "cancelled"]

export function isTerminalJobStatus(status: string): boolean {
  return TERMINAL_JOB_STATUSES.includes(status.toLowerCase())
}

// ── IAM plane — Roles, API keys, Audit (PascalCase) ──────────────────────────

export type Permission =
  | "lists:get"
  | "lists:manage"
  | "subscribers:get"
  | "subscribers:manage"
  | "subscribers:import"
  | "subscribers:export"
  | "roles:get"
  | "roles:manage"
  | "apikeys:get"
  | "apikeys:manage"
  | "audit:get"
  | "settings:get"
  | "settings:manage"

export const ALL_PERMISSIONS: Array<Permission> = [
  "lists:get",
  "lists:manage",
  "subscribers:get",
  "subscribers:manage",
  "subscribers:import",
  "subscribers:export",
  "roles:get",
  "roles:manage",
  "apikeys:get",
  "apikeys:manage",
  "audit:get",
  "settings:get",
  "settings:manage",
]

export type Role = {
  ID: string
  Name: string
  Permissions: Array<Permission>
  CreatedAt: string
  UpdatedAt: string
}

export type APIKey = {
  ID: string
  Name: string
  Permissions: Array<Permission>
  CreatedAt: string
  LastUsedAt: string | null
  RevokedAt: string | null
}

export type IssuedAPIKey = {
  id: string
  token: string
}

export type AuditRecord = {
  ID: string
  ActorID: string
  ActorKind: string
  Action: string
  Target: string
  Metadata: Record<string, unknown>
  CreatedAt: string
}

// ── TOTP enrolment ───────────────────────────────────────────────────────────

export type TOTPEnrolment = {
  secret: string
  uri: string
}

export type TOTPConfirmation = {
  recovery_codes: Array<string>
}

// ── Paging ───────────────────────────────────────────────────────────────────

export type Page = {
  limit: number
  offset: number
}

export const DEFAULT_PAGE_SIZE = 25

export type Paged<T> = {
  items: Array<T>
  total: number
}

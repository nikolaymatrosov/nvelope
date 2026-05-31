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
  // The user's chosen interface language; null until they pick one.
  locale: string | null
}

// Body of PUT /api/platform/me — updates the caller's language preference.
export type AccountLocaleInput = {
  locale: string
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
  | "sending:get"
  | "sending:manage"
  | "campaigns:get"
  | "campaigns:manage"
  | "transactional:send"
  | "billing:get"
  | "billing:manage"
  | "subscription_pages:manage"
  | "branding:manage"
  | "media:get"
  | "media:manage"
  | "subscriber_fields:manage"

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
  "sending:get",
  "sending:manage",
  "campaigns:get",
  "campaigns:manage",
  "transactional:send",
  "billing:get",
  "billing:manage",
  "subscription_pages:manage",
  "branding:manage",
  "media:get",
  "media:manage",
  "subscriber_fields:manage",
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

// ── Sending domains (Phase 3, snake_case) ────────────────────────────────────

export type DomainStatus = "pending" | "verified" | "failed"

export type DNSRecord = {
  type: string
  name: string
  value: string
}

export type DomainView = {
  id: string
  domain: string
  status: DomainStatus
  dkim_records: Array<DNSRecord>
  spf_record: string
  dmarc_record: string
  failure_reason?: string
  created_at: string
  verified_at?: string
  last_checked_at?: string
}

// ── Templates (Phase 3, snake_case) ──────────────────────────────────────────

export type TemplateKind = "campaign" | "transactional"

export type TemplateView = {
  id: string
  name: string
  kind: TemplateKind
  subject: string
  body_html: string
  body_text: string
  // Phase 7 — populated when the template was last saved visually.
  body_doc?: VisualDoc | null
  theme?: Theme | null
  created_at: string
  updated_at: string
}

export type CreateTemplateInput = {
  name: string
  kind: TemplateKind
  subject: string
  body_html: string
  body_text: string
}

export type UpdateTemplateInput = {
  name: string
  subject: string
  body_html: string
  body_text: string
}

// ── Campaigns (Phase 3, snake_case) ──────────────────────────────────────────

export type CampaignStatus =
  | "draft"
  | "running"
  | "paused"
  | "finished"
  | "cancelled"

export const TERMINAL_CAMPAIGN_STATUSES: Array<CampaignStatus> = [
  "finished",
  "cancelled",
]

export function isTerminalCampaignStatus(status: string): boolean {
  return TERMINAL_CAMPAIGN_STATUSES.includes(status as CampaignStatus)
}

export type CampaignView = {
  id: string
  name: string
  subject: string
  body_html: string
  body_text: string
  // Phase 7 — populated when the campaign was last saved via the visual
  // editor. NULL means raw-HTML / code-only — the operator must opt-in
  // to switch to the visual editor (per FR-029).
  body_doc?: VisualDoc | null
  theme?: Theme | null
  from_name: string
  from_local_part: string
  sending_domain_id?: string
  template_id?: string
  status: CampaignStatus
  max_send_errors: number
  sent_count: number
  failed_count: number
  recipient_count: number
  list_ids: Array<string>
  segments: Array<Node> | null
  created_at: string
  updated_at: string
  started_at?: string
  finished_at?: string
  archive_visible?: boolean
}

export type CreateCampaignInput = {
  name: string
  template_id?: string
  subject: string
  body_html: string
  body_text: string
  from_name: string
  from_local_part: string
  sending_domain_id?: string
  list_ids: Array<string>
  segments?: Array<Node>
  max_send_errors?: number
}

// ── Deliverability & Analytics (Phase 4, camelCase) ──────────────────────────

export type SuppressionReason = "hard_bounce" | "complaint" | "manual"

export type SuppressionEntry = {
  email: string
  reason: SuppressionReason
  suppressedAt: string
  note: string
}

export type SuppressionListResponse = {
  items: Array<SuppressionEntry>
  nextCursor: string | null
}

export type BounceSettings = {
  suppressOnHardBounce: boolean
  suppressOnComplaint: boolean
}

export type DeliveryCounts = {
  sent: number
  delivered: number
  opened: number
  clicked: number
  bounced: number
  complained: number
}

export type CampaignRates = {
  openRate: number
  clickRate: number
  bounceRate: number
  complaintRate: number
}

export type CampaignAnalytics = {
  campaignId: string
  counts: DeliveryCounts
  rates: CampaignRates
  refreshedAt: string | null
}

export type RecentCampaign = {
  campaignId: string
  name: string
  sent: number
  openRate: number
  bounceRate: number
  complaintRate: number
}

export type DashboardView = {
  totals: DeliveryCounts
  deliverability: {
    bounceRate: number
    complaintRate: number
  }
  recentCampaigns: Array<RecentCampaign>
}

export type UpdateCampaignInput = {
  name: string
  subject: string
  body_html: string
  body_text: string
  from_name: string
  from_local_part: string
  sending_domain_id?: string
  list_ids: Array<string>
  segments?: Array<Node>
}

// ── Billing & Metering (Phase 5, camelCase) ──────────────────────────────────

export type OverageMode = "block" | "meter"

export type SubscriptionState =
  | "pending"
  | "active"
  | "past_due"
  | "suspended"
  | "cancelled"

export type InvoiceStatus = "open" | "paid" | "void"

// A plan as presented in the catalogue (GET /plans).
export type PlanView = {
  id: string
  code: string
  name: string
  priceMinor: number
  currency: string
  billingPeriod: string
  includedSends: number
  overageMode: OverageMode
  overagePriceMinor: number
}

// The plan summary embedded in a subscription (GET /subscription).
export type PlanRef = {
  id: string
  code: string
  name: string
  overageMode: OverageMode
}

export type SubscriptionView = {
  id: string
  plan: PlanRef
  state: SubscriptionState
  currentPeriodStart: string
  currentPeriodEnd: string
  cancelAtPeriodEnd: boolean
}

export type UsageView = {
  includedSends: number
  usedSends: number
  overageSends: number
  remainingSends: number
}

// GET /subscription envelope — subscription + current-period usage.
export type SubscriptionResponse = {
  subscription: SubscriptionView
  usage: UsageView
}

// POST /subscription result — the new subscription plus its first invoice.
export type SubscribeResult = {
  subscription: {
    id: string
    planId: string
    state: SubscriptionState
    currentPeriodStart: string
    currentPeriodEnd: string
    cancelAtPeriodEnd: boolean
  }
  invoice: {
    id: string
    status: InvoiceStatus
    totalMinor: number
    currency: string
  }
}

// An invoice row as listed in the history (GET /invoices).
export type InvoiceSummary = {
  id: string
  periodStart: string
  periodEnd: string
  totalMinor: number
  currency: string
  status: InvoiceStatus
  issuedAt: string | null
  paidAt: string | null
}

export type LineItemView = {
  kind: string
  description: string
  quantity: number
  unitPriceMinor: number
  amountMinor: number
}

export type PaymentAttemptView = {
  attemptNumber: number
  status: "succeeded" | "failed"
  gatewayReference: string
  failureReason: string
  createdAt: string
}

// A full invoice with line items and payment attempts (GET /invoices/{id}).
export type InvoiceView = InvoiceSummary & {
  subscriptionId: string
  attemptCount: number
  nextAttemptAt: string | null
  lineItems: Array<LineItemView>
  paymentAttempts: Array<PaymentAttemptView>
}

// ── Public pages & media (Phase 6) ───────────────────────────────────────────

// One configurable field beyond the always-present email a public subscription
// page collects. Matches `audiencedomain.FormField` (snake_case json tags).
export type SubscriptionPageFieldView = {
  key: string
  label: string
  required: boolean
}

// SubscriptionPageView matches the backend's PascalCase read model (no JSON
// tags on the Go struct) — fields are emitted as-is.
export type SubscriptionPageView = {
  ID: string
  Slug: string
  Title: string
  TargetListIDs: Array<string>
  Fields: Array<SubscriptionPageFieldView>
  SendingDomainID: string
  FromName: string
  FromLocalPart: string
  Active: boolean
  CreatedAt: string
  UpdatedAt: string
}

export type SaveSubscriptionPageInput = {
  slug: string
  title: string
  target_list_ids: Array<string>
  fields: Array<SubscriptionPageFieldView>
  sending_domain_id: string
  from_name: string
  from_local_part: string
  active: boolean
}

// BrandingView matches the backend's snake_case json shape.
export type BrandingView = {
  logo_url: string
  primary_color: string
  custom_css: string
}

export type SaveBrandingInput = {
  logo_url: string
  primary_color: string
  custom_css: string
}

// The custom-CSS limit is enforced server-side; the UI uses this constant for
// inline byte-length feedback. Bumping it requires a backend change too.
export const CUSTOM_CSS_LIMIT_BYTES = 16384

// MediaAssetView matches the backend's snake_case json shape.
export type MediaAssetView = {
  id: string
  filename: string
  content_type: string
  size_bytes: number
  public_url: string
  uploaded_by?: string
  created_at: string
}

// Result returned by POST /media — narrower than the full view.
export type MediaUploadResult = {
  id: string
  public_url: string
  filename: string
}

// Mirror of the backend's `media.domain.allowedContentTypes` allowlist. The
// server is authoritative; the UI uses this for early rejection only.
export const ALLOWED_MEDIA_CONTENT_TYPES: Array<string> = [
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
  "image/svg+xml",
  "application/pdf",
]

// Default upload size cap mirroring the backend default
// (`config.MediaMaxBytes` default = 10 MB). Used only for early rejection
// before sending; the backend still enforces the configured limit.
export const DEFAULT_MEDIA_MAX_BYTES = 10 * 1024 * 1024

export function isImageContentType(contentType: string): boolean {
  return contentType.startsWith("image/")
}

// ── Visual editor (Phase 7) ──────────────────────────────────────────────────
//
// Wire shape for the structured-document editor. Mirrors the Go
// `internal/campaign/domain.VisualDoc` types and the BFF render tier's
// `frontend/src/server/render/types.ts`. The SPA produces VisualDoc JSON in
// the browser; the BFF renders it to HTML+text; Go validates, sanitizes,
// and persists. Field names match the wire format byte-for-byte — see
// `specs/014-visual-email-editor/contracts/tenant-api.md` §
// "Structured-document JSON schema".

export type MergeTagNamespace = "subscriber" | "campaign"

// BlockStyle — the optional, email-safe per-block style the three-pane editor's
// parameters panel produces (feature 017). Mirrors BlockStyle in the BFF render
// tier (frontend/src/server/render/types.ts) and the Go domain. Absent field ⇒
// inherit the document theme / default.
export type BlockStyle = {
  backgroundColor?: string
  color?: string
  fontFamily?: string
  fontSize?: number
  fontWeight?: 400 | 700
  lineHeight?: number
  textAlign?: "left" | "center" | "right"
  paddingTop?: number
  paddingRight?: number
  paddingBottom?: number
  paddingLeft?: number
  borderRadius?: number
  borderWidth?: number
  borderStyle?: "solid" | "dashed" | "dotted"
  borderColor?: string
}

export type Mark =
  | { type: "bold" }
  | { type: "italic" }
  | { type: "underline" }
  | { type: "strike" }
  | { type: "color"; attrs: { color: string } }
  | { type: "link"; attrs: { href: string } }

export type TextInline = {
  type: "text"
  text: string
  marks?: Array<Mark>
}

export type MergeTagInline = {
  type: "mergeTag"
  attrs: { namespace: MergeTagNamespace; key: string }
}

export type Inline = TextInline | MergeTagInline

export type ParagraphBlock = {
  type: "paragraph"
  attrs?: { style?: BlockStyle }
  content: Array<Inline>
}
export type HeadingBlock = {
  type: "heading"
  attrs: { level: 1 | 2 | 3; style?: BlockStyle }
  content: Array<Inline>
}
export type ListItemBlock = { type: "listItem"; content: Array<VisualBlock> }
export type BulletListBlock = {
  type: "bulletList"
  attrs?: { style?: BlockStyle }
  content: Array<ListItemBlock>
}
export type OrderedListBlock = {
  type: "orderedList"
  attrs?: { style?: BlockStyle }
  content: Array<ListItemBlock>
}
export type BlockquoteBlock = {
  type: "blockquote"
  attrs?: { style?: BlockStyle }
  content: Array<VisualBlock>
}
export type CodeBlock = {
  type: "codeBlock"
  content: Array<{ type: "text"; text: string }>
}
export type ImageBlock = {
  type: "image"
  attrs: { mediaRef: string; alt: string; href: string; style?: BlockStyle }
}
export type ButtonBlock = {
  type: "button"
  attrs: { label: string; href: string; style?: BlockStyle }
}
export type DividerBlock = { type: "divider"; attrs?: { style?: BlockStyle } }
export type ColumnBlock = {
  type: "column"
  attrs?: { style?: BlockStyle }
  content: Array<VisualBlock>
}
export type ColumnsBlock = {
  type: "columns"
  attrs: { count: 2 | 3 | 4; style?: BlockStyle }
  content: Array<ColumnBlock>
}
export type RawHtmlBlock = { type: "rawHtml"; attrs: { html: string } }

export type VisualBlock =
  | ParagraphBlock
  | HeadingBlock
  | BulletListBlock
  | OrderedListBlock
  | ListItemBlock
  | BlockquoteBlock
  | CodeBlock
  | ImageBlock
  | ButtonBlock
  | DividerBlock
  | ColumnsBlock
  | ColumnBlock
  | RawHtmlBlock

export type VisualDoc = {
  version: 1
  type: "doc"
  content: Array<VisualBlock>
}

// Theme value object. NULL on the row means "inherit tenant branding".
export type Theme = {
  textColor: string
  linkColor: string
  buttonColor: string
  buttonTextColor: string
  fontFamily: string
  containerWidth: number
}

export type RenderWarning = {
  kind: string
  detail: string
}

// Subscriber-field registry — surfaced as one merged list by
// GET /subscriber-fields (built-in pseudo-rows + tenant rows).
export type FieldType = "text" | "number" | "date" | "boolean" | "url"

export const FIELD_TYPES: Array<FieldType> = [
  "text",
  "number",
  "date",
  "boolean",
  "url",
]

export type Field = {
  id: string
  slug: string
  displayName: string
  type: FieldType
  defaultValue: string
  position: number
  builtIn: boolean
}

export type CreateFieldInput = {
  slug: string
  displayName: string
  type: FieldType
  defaultValue: string
}

export type UpdateFieldInput = {
  displayName?: string
  type?: FieldType
  defaultValue?: string
}

// Merge-tag picker — one shape combining subscriber + campaign-namespace
// entries returned by GET /merge-tags.
export type MergeTagSubscriberItem = {
  slug: string
  displayName: string
  type: FieldType
  builtIn: boolean
}

export type MergeTagCampaignItem = {
  key: string
  displayName: string
}

export type MergeTagPickerItem =
  | ({ namespace: "subscriber" } & MergeTagSubscriberItem)
  | ({ namespace: "campaign" } & MergeTagCampaignItem)

export type MergeTagsResponse = {
  subscriber: Array<MergeTagSubscriberItem>
  campaign: Array<MergeTagCampaignItem>
}

// Visual save — campaigns.
export type SaveVisualCampaignInput = {
  subject: string
  bodyDoc: VisualDoc
  theme: Theme | null
  ifUnmodifiedSince: string
}

// Visual save — templates.
export type SaveVisualTemplateInput = {
  name: string
  kind: TemplateKind
  subject: string
  bodyDoc: VisualDoc
  theme: Theme | null
  ifUnmodifiedSince: string
}

// Successful visual save response. Campaign and template responses share
// this shape (template adds name+kind on top — captured in the
// VisualTemplateSaveResponse extension below).
export type VisualSaveResponse = {
  id: string
  subject: string
  bodyHtml: string
  bodyText: string
  bodyDoc: VisualDoc
  theme: Theme | null
  warnings: Array<RenderWarning>
  updatedAt: string
}

export type VisualTemplateSaveResponse = VisualSaveResponse & {
  name: string
  kind: TemplateKind
}

// Render-preview — tenant-scoped (no row id). Shared by campaign + template
// editors. Optional `sample` triggers a Go-side substitute-sample side-call.
export type RenderPreviewInput = {
  bodyDoc: VisualDoc
  theme: Theme | null
  sample: RenderPreviewSample | null
}

export type RenderPreviewSample = {
  subscriber: Record<string, string>
  campaign: Record<string, string>
}

export type RenderPreviewResponse = {
  bodyHtml: string
  bodyText: string
  warnings: Array<RenderWarning>
}

// Stale-row 409 payload — surfaced when ifUnmodifiedSince diverges from
// the row's current updated_at. The SPA echoes currentUpdatedAt on the
// "Force overwrite" path.
export type StaleRowError = {
  kind: "stale_row"
  currentUpdatedAt: string
}

// Convert-to-visual — non-persisting response from
// POST /campaigns/{id}/convert-to-visual and the templates counterpart.
// `bodyDoc` is the candidate VisualDoc the converter produced;
// `warnings` lists every RawHTML fallback the operator should review
// before saving with the regular visual PUT.
export type ConvertToVisualResponse = {
  bodyDoc: VisualDoc
  warnings: Array<ConvertWarning>
}

export type ConvertWarning = {
  kind: "rawhtml_block"
  detail: string
  path: string
}

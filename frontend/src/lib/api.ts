// Typed client for the nvelope platform and tenant APIs — the single transport
// layer every screen depends on (Constitution Principle VI). Paths are relative
// so the browser sends the session cookie same-origin; the Vite dev server
// proxies /api and /t/{slug}/api to the Go API service.
//
// Every method resolves to `ApiResult<T>` on success and raises `ApiError`
// (normalized by src/lib/errors.ts) on any non-2xx response. Tenant-scoped
// methods take `slug` as their first argument so a call site cannot omit it
// (tenant-isolation safety, Principle I).

import { ApiError, normalizeError } from "./errors"
import type {
  APIKey,
  AuditRecord,
  BounceSettings,
  BrandingView,
  CampaignAnalytics,
  CampaignView,
  ConvertToVisualResponse,
  CreateCampaignInput,
  CreateFieldInput,
  CreateListInput,
  CreateSubscriberInput,
  CreateTemplateInput,
  DashboardView,
  DomainView,
  Field,
  InvitationLookup,
  InvoiceSummary,
  InvoiceView,
  IssuedAPIKey,
  JobStatusView,
  List,
  MediaAssetView,
  MediaUploadResult,
  Membership,
  MergeTagsResponse,
  Node,
  Page,
  Permission,
  PlanView,
  PlatformAccount,
  PlatformUser,
  RenderPreviewInput,
  RenderPreviewResponse,
  Role,
  SaveBrandingInput,
  SaveSubscriptionPageInput,
  SaveVisualCampaignInput,
  SaveVisualTemplateInput,
  SessionResult,
  StartExportInput,
  SubscribeResult,
  Subscriber,
  SubscriptionPageView,
  SubscriptionResponse,
  SuppressionEntry,
  SuppressionListResponse,
  SuppressionReason,
  TOTPConfirmation,
  TOTPEnrolment,
  TemplateView,
  UpdateCampaignInput,
  UpdateFieldInput,
  UpdateListInput,
  UpdateSubscriberInput,
  UpdateTemplateInput,
  VisualSaveResponse,
  VisualTemplateSaveResponse,
  WorkspaceInfo,
  WorkspaceInvitation,
  WorkspaceSettings,
} from "./api-types"

const BASE: string = import.meta.env.VITE_API_BASE ?? ""

type Json = Record<string, unknown> | undefined

export type ApiResult<T = unknown> = { status: number; ok: boolean; data: T }

export type SuppressionListParams = {
  cursor?: string
  limit?: number
  reason?: SuppressionReason
  email?: string
}

async function parseBody(res: Response): Promise<unknown> {
  const text = await res.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

async function request<T = unknown>(
  method: string,
  path: string,
  body?: Json,
): Promise<ApiResult<T>> {
  const res = await fetch(BASE + path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    credentials: "include",
  })
  const data = await parseBody(res)
  if (!res.ok) throw normalizeError(res.status, data, path)
  return { status: res.status, ok: res.ok, data: data as T }
}

async function requestMultipart<T = unknown>(
  method: string,
  path: string,
  form: FormData,
): Promise<ApiResult<T>> {
  const res = await fetch(BASE + path, {
    method,
    body: form,
    credentials: "include",
  })
  const data = await parseBody(res)
  if (!res.ok) throw normalizeError(res.status, data, path)
  return { status: res.status, ok: res.ok, data: data as T }
}

function pageQuery(page?: Page): string {
  if (!page) return ""
  return `?limit=${page.limit}&offset=${page.offset}`
}

const tp = (slug: string, suffix: string) => `/t/${slug}/api${suffix}`

export const api = {
  // ── Platform plane ─────────────────────────────────────────────────────────
  signup: (email: string, password: string, name: string) =>
    request<PlatformAccount>("POST", "/api/platform/signup", {
      email,
      password,
      name,
    }),
  login: (email: string, password: string) =>
    request("POST", "/api/platform/login", { email, password }),
  logout: () => request("POST", "/api/platform/logout"),
  me: () => request<PlatformAccount>("GET", "/api/platform/me"),
  updateMyLocale: (locale: string) =>
    request<{ user: PlatformUser }>("PUT", "/api/platform/me", { locale }),
  createTenant: (name: string, slug: string) =>
    request<{ id: string }>("POST", "/api/platform/tenants", { name, slug }),
  listTenants: () =>
    request<{ tenants: Array<Membership> }>("GET", "/api/platform/tenants"),
  getInvitation: (token: string) =>
    request<InvitationLookup>(
      "GET",
      `/api/platform/invitations/${token}`,
    ),
  acceptInvitation: (token: string, password?: string, name?: string) =>
    request<{ tenant: { slug: string } }>(
      "POST",
      `/api/platform/invitations/${token}/accept`,
      password !== undefined ? { password, name } : undefined,
    ),

  // ── Workspace, settings, invitations ───────────────────────────────────────
  tenant: (slug: string) =>
    request<WorkspaceInfo>("GET", tp(slug, "/tenant")),
  getSettings: (slug: string) =>
    request<WorkspaceSettings>("GET", tp(slug, "/settings")),
  updateSettings: (slug: string, body: Partial<WorkspaceSettings>) =>
    request("PUT", tp(slug, "/settings"), body),
  invite: (slug: string, email: string) =>
    request<{ accept_url: string }>("POST", tp(slug, "/invitations"), { email }),
  listInvitations: (slug: string) =>
    request<{ invitations: Array<WorkspaceInvitation> }>(
      "GET",
      tp(slug, "/invitations"),
    ),
  revokeInvitation: (slug: string, id: string) =>
    request("DELETE", tp(slug, `/invitations/${id}`)),

  // ── Workspace session ──────────────────────────────────────────────────────
  openSession: (slug: string) =>
    request<SessionResult>("POST", tp(slug, "/session")),
  closeSession: (slug: string) =>
    request("DELETE", tp(slug, "/session")),
  verifySessionTOTP: (slug: string, code: string) =>
    request<SessionResult>("POST", tp(slug, "/session/totp"), { code }),

  // ── Lists ──────────────────────────────────────────────────────────────────
  createList: (slug: string, body: CreateListInput) =>
    request<{ id: string }>("POST", tp(slug, "/lists"), body),
  listLists: (slug: string, page?: Page) =>
    request<{ lists: Array<List>; total: number }>(
      "GET",
      tp(slug, `/lists${pageQuery(page)}`),
    ),
  getList: (slug: string, id: string) =>
    request<{ list: List }>("GET", tp(slug, `/lists/${id}`)),
  updateList: (slug: string, id: string, body: UpdateListInput) =>
    request("PUT", tp(slug, `/lists/${id}`), body),
  deleteList: (slug: string, id: string) =>
    request("DELETE", tp(slug, `/lists/${id}`)),

  // ── Subscribers ────────────────────────────────────────────────────────────
  createSubscriber: (slug: string, body: CreateSubscriberInput) =>
    request<{ id: string }>("POST", tp(slug, "/subscribers"), body),
  searchSubscribers: (slug: string, q: string, page?: Page) =>
    request<{ subscribers: Array<Subscriber>; total: number }>(
      "GET",
      tp(
        slug,
        `/subscribers?q=${encodeURIComponent(q)}${
          page ? `&limit=${page.limit}&offset=${page.offset}` : ""
        }`,
      ),
    ),
  querySubscribers: (slug: string, segment: Node, page?: Page) =>
    request<{ subscribers: Array<Subscriber>; total: number }>(
      "POST",
      tp(slug, `/subscribers/query${pageQuery(page)}`),
      { segment },
    ),
  countSubscribers: (slug: string, segment: Node) =>
    request<{ total: number }>(
      "POST",
      tp(slug, "/subscribers/query/count"),
      { segment },
    ),
  getSubscriber: (slug: string, id: string) =>
    request<{ subscriber: Subscriber }>(
      "GET",
      tp(slug, `/subscribers/${id}`),
    ),
  updateSubscriber: (slug: string, id: string, body: UpdateSubscriberInput) =>
    request("PUT", tp(slug, `/subscribers/${id}`), body),
  deleteSubscriber: (slug: string, id: string) =>
    request("DELETE", tp(slug, `/subscribers/${id}`)),
  addToList: (slug: string, id: string, listId: string) =>
    request("POST", tp(slug, `/subscribers/${id}/lists`), { list_id: listId }),
  removeFromList: (slug: string, id: string, listId: string) =>
    request("DELETE", tp(slug, `/subscribers/${id}/lists/${listId}`)),
  changeSubscription: (
    slug: string,
    id: string,
    listId: string,
    status: string,
  ) =>
    request("PUT", tp(slug, `/subscribers/${id}/lists/${listId}`), { status }),

  // ── Import / export jobs ───────────────────────────────────────────────────
  startImport: (slug: string, file: File, listIds: Array<string>) => {
    const form = new FormData()
    form.append("file", file)
    for (const id of listIds) form.append("list_ids", id)
    return requestMultipart<{ job_id: string }>(
      "POST",
      tp(slug, "/import"),
      form,
    )
  },
  startExport: (slug: string, body: StartExportInput) =>
    request<{ job_id: string }>("POST", tp(slug, "/export"), { ...body }),
  jobStatus: (slug: string, id: string) =>
    request<JobStatusView>("GET", tp(slug, `/jobs/${id}`)),
  downloadExportUrl: (slug: string, id: string) =>
    BASE + tp(slug, `/jobs/${id}/download`),

  // ── Roles & assignments ────────────────────────────────────────────────────
  createRole: (slug: string, name: string, permissions: Array<Permission>) =>
    request<{ id: string }>("POST", tp(slug, "/roles"), { name, permissions }),
  listRoles: (slug: string) =>
    request<{ roles: Array<Role> }>("GET", tp(slug, "/roles")),
  updateRole: (
    slug: string,
    id: string,
    body: { name: string; permissions: Array<Permission> },
  ) => request("PUT", tp(slug, `/roles/${id}`), body),
  deleteRole: (slug: string, id: string) =>
    request("DELETE", tp(slug, `/roles/${id}`)),
  assignRole: (slug: string, userId: string, roleId: string) =>
    request("PUT", tp(slug, `/users/${userId}/role`), { role_id: roleId }),
  assignListRole: (
    slug: string,
    userId: string,
    listId: string,
    roleId: string,
  ) =>
    request("PUT", tp(slug, `/users/${userId}/lists/${listId}/role`), {
      role_id: roleId,
    }),
  removeListRole: (slug: string, userId: string, listId: string) =>
    request("DELETE", tp(slug, `/users/${userId}/lists/${listId}/role`)),

  // ── API keys ───────────────────────────────────────────────────────────────
  issueAPIKey: (slug: string, name: string, permissions: Array<Permission>) =>
    request<IssuedAPIKey>("POST", tp(slug, "/api-keys"), {
      name,
      permissions,
    }),
  listAPIKeys: (slug: string) =>
    request<{ api_keys: Array<APIKey> }>("GET", tp(slug, "/api-keys")),
  revokeAPIKey: (slug: string, id: string) =>
    request("DELETE", tp(slug, `/api-keys/${id}`)),

  // ── TOTP enrolment & audit ─────────────────────────────────────────────────
  enableTOTP: (slug: string) =>
    request<TOTPEnrolment>("POST", tp(slug, "/me/totp")),
  confirmTOTP: (slug: string, secret: string, code: string) =>
    request<TOTPConfirmation>("POST", tp(slug, "/me/totp/confirm"), {
      secret,
      code,
    }),
  disableTOTP: (slug: string) =>
    request("DELETE", tp(slug, "/me/totp")),
  auditTrail: (slug: string, page?: Page) =>
    request<{ records: Array<AuditRecord>; total: number }>(
      "GET",
      tp(slug, `/audit${pageQuery(page)}`),
    ),

  // ── Sending domains (Phase 3) ──────────────────────────────────────────────
  addSendingDomain: (slug: string, domain: string) =>
    request<DomainView>("POST", tp(slug, "/sending-domains"), { domain }),
  listSendingDomains: (slug: string) =>
    request<{ domains: Array<DomainView> }>(
      "GET",
      tp(slug, "/sending-domains"),
    ),
  getSendingDomain: (slug: string, id: string) =>
    request<DomainView>("GET", tp(slug, `/sending-domains/${id}`)),
  recheckSendingDomain: (slug: string, id: string) =>
    request<{ status: string }>(
      "POST",
      tp(slug, `/sending-domains/${id}/recheck`),
    ),

  // ── Templates (Phase 3) ────────────────────────────────────────────────────
  createTemplate: (slug: string, body: CreateTemplateInput) =>
    request<TemplateView>("POST", tp(slug, "/templates"), { ...body }),
  listTemplates: (slug: string, page?: Page) =>
    request<{ templates: Array<TemplateView>; total: number }>(
      "GET",
      tp(slug, `/templates${pageQuery(page)}`),
    ),
  getTemplate: (slug: string, id: string) =>
    request<TemplateView>("GET", tp(slug, `/templates/${id}`)),
  updateTemplate: (slug: string, id: string, body: UpdateTemplateInput) =>
    request<TemplateView>("PUT", tp(slug, `/templates/${id}`), { ...body }),
  deleteTemplate: (slug: string, id: string) =>
    request("DELETE", tp(slug, `/templates/${id}`)),

  // ── Campaigns (Phase 3) ────────────────────────────────────────────────────
  createCampaign: (slug: string, body: CreateCampaignInput) =>
    request<CampaignView>("POST", tp(slug, "/campaigns"), { ...body }),
  listCampaigns: (slug: string, page?: Page) =>
    request<{ campaigns: Array<CampaignView>; total: number }>(
      "GET",
      tp(slug, `/campaigns${pageQuery(page)}`),
    ),
  getCampaign: (slug: string, id: string) =>
    request<CampaignView>("GET", tp(slug, `/campaigns/${id}`)),
  updateCampaign: (slug: string, id: string, body: UpdateCampaignInput) =>
    request<CampaignView>("PUT", tp(slug, `/campaigns/${id}`), { ...body }),
  startCampaign: (slug: string, id: string) =>
    request<{ status: string }>("POST", tp(slug, `/campaigns/${id}/start`)),
  pauseCampaign: (slug: string, id: string) =>
    request<{ status: string }>("POST", tp(slug, `/campaigns/${id}/pause`)),
  resumeCampaign: (slug: string, id: string) =>
    request<{ status: string }>("POST", tp(slug, `/campaigns/${id}/resume`)),
  cancelCampaign: (slug: string, id: string) =>
    request<{ status: string }>("POST", tp(slug, `/campaigns/${id}/cancel`)),
  setCampaignArchive: (slug: string, id: string, visible: boolean) =>
    request<{ visible: boolean }>(
      "POST",
      tp(slug, `/campaigns/${id}/archive`),
      { visible },
    ),

  // ── Suppression list (Phase 4) ─────────────────────────────────────────────
  suppressions: {
    list: (slug: string, params?: SuppressionListParams) => {
      const q = new URLSearchParams()
      if (params?.cursor) q.set("cursor", params.cursor)
      if (params?.limit) q.set("limit", String(params.limit))
      if (params?.reason) q.set("reason", params.reason)
      if (params?.email) q.set("email", params.email)
      const qs = q.toString()
      return request<SuppressionListResponse>(
        "GET",
        tp(slug, `/suppressions${qs ? `?${qs}` : ""}`),
      )
    },
    add: (slug: string, email: string, note = "") =>
      request<SuppressionEntry>("POST", tp(slug, "/suppressions"), {
        email,
        note,
      }),
    remove: (slug: string, email: string) =>
      request(
        "DELETE",
        tp(slug, `/suppressions/${encodeURIComponent(email)}`),
      ),
  },

  // ── Bounce-action settings (Phase 4) ───────────────────────────────────────
  bounceSettings: {
    get: (slug: string) =>
      request<BounceSettings>("GET", tp(slug, "/bounce-settings")),
    update: (slug: string, body: BounceSettings) =>
      request<BounceSettings>("PUT", tp(slug, "/bounce-settings"), body),
  },

  // ── Analytics & dashboard (Phase 4) ────────────────────────────────────────
  analytics: {
    campaign: (slug: string, id: string) =>
      request<CampaignAnalytics>(
        "GET",
        tp(slug, `/campaigns/${id}/analytics`),
      ),
    dashboard: (slug: string) =>
      request<DashboardView>("GET", tp(slug, "/dashboard")),
  },

  // ── Billing & metering (Phase 5) ───────────────────────────────────────────
  billing: {
    plans: (slug: string) =>
      request<{ plans: Array<PlanView> }>("GET", tp(slug, "/plans")),
    getSubscription: (slug: string) =>
      request<SubscriptionResponse>("GET", tp(slug, "/subscription")),
    subscribe: (slug: string, planId: string) =>
      request<SubscribeResult>("POST", tp(slug, "/subscription"), { planId }),
    cancelSubscription: (slug: string) =>
      request("DELETE", tp(slug, "/subscription")),
    listInvoices: (slug: string, limit: number, offset: number) =>
      request<{ invoices: Array<InvoiceSummary>; total: number }>(
        "GET",
        tp(slug, `/invoices?limit=${limit}&offset=${offset}`),
      ),
    getInvoice: (slug: string, id: string) =>
      request<InvoiceView>("GET", tp(slug, `/invoices/${id}`)),
    settleInvoice: (slug: string, id: string) =>
      request<InvoiceView>("POST", tp(slug, `/invoices/${id}/settle`)),
  },

  // ── Subscription pages (Phase 6) ───────────────────────────────────────────
  subscriptionPages: {
    list: (slug: string) =>
      request<{ subscription_pages: Array<SubscriptionPageView> }>(
        "GET",
        tp(slug, "/subscription-pages"),
      ),
    create: (slug: string, body: SaveSubscriptionPageInput) =>
      request<SubscriptionPageView>(
        "POST",
        tp(slug, "/subscription-pages"),
        { ...body },
      ),
    update: (slug: string, id: string, body: SaveSubscriptionPageInput) =>
      request<SubscriptionPageView>(
        "PUT",
        tp(slug, `/subscription-pages/${id}`),
        { ...body },
      ),
  },

  // ── Branding (Phase 6) ─────────────────────────────────────────────────────
  branding: {
    get: (slug: string) =>
      request<BrandingView>("GET", tp(slug, "/branding")),
    save: (slug: string, body: SaveBrandingInput) =>
      request<BrandingView>("PUT", tp(slug, "/branding"), { ...body }),
  },

  // ── Subscriber-field registry (Phase 7) ────────────────────────────────────
  subscriberFields: {
    list: (slug: string) =>
      request<{ fields: Array<Field> }>(
        "GET",
        tp(slug, "/subscriber-fields"),
      ),
    create: (slug: string, body: CreateFieldInput) =>
      request<Field>("POST", tp(slug, "/subscriber-fields"), { ...body }),
    update: (slug: string, id: string, body: UpdateFieldInput) =>
      request<Field>("PATCH", tp(slug, `/subscriber-fields/${id}`), {
        ...body,
      }),
    delete: (slug: string, id: string) =>
      request("DELETE", tp(slug, `/subscriber-fields/${id}`)),
    reorder: (slug: string, order: Array<string>) =>
      request<{ fields: Array<Field> }>(
        "PATCH",
        tp(slug, "/subscriber-fields/order"),
        { order },
      ),
  },

  // ── Merge-tag picker (Phase 7) ─────────────────────────────────────────────
  mergeTags: {
    list: (slug: string) =>
      request<MergeTagsResponse>("GET", tp(slug, "/merge-tags")),
  },

  // ── Visual editor saves & preview (Phase 7) ────────────────────────────────
  // The two visual-save endpoints and render-preview are hosted by the BFF
  // (Nitro), not by Go directly — see specs/014-visual-email-editor/
  // contracts/tenant-api.md § "Hosting tier note". The browser still uses
  // the same /t/{slug}/api/… URL space; Nitro intercepts these three paths
  // before the catch-all proxy. The 409 stale_row error envelope carries
  // `{ kind: "stale_row", currentUpdatedAt }` so callers can present the
  // Reload / Force-overwrite affordance per FR-009.
  campaigns: {
    saveVisual: (
      slug: string,
      id: string,
      body: SaveVisualCampaignInput,
    ) =>
      request<VisualSaveResponse>(
        "PUT",
        tp(slug, `/campaigns/${id}/visual`),
        { ...body },
      ),
    convertToVisual: (slug: string, id: string) =>
      request<ConvertToVisualResponse>(
        "POST",
        tp(slug, `/campaigns/${id}/convert-to-visual`),
      ),
    optOutVisual: (slug: string, id: string) =>
      request<CampaignView>(
        "POST",
        tp(slug, `/campaigns/${id}/opt-out-visual`),
      ),
  },
  templates: {
    saveVisual: (
      slug: string,
      id: string,
      body: SaveVisualTemplateInput,
    ) =>
      request<VisualTemplateSaveResponse>(
        "PUT",
        tp(slug, `/templates/${id}/visual`),
        { ...body },
      ),
    convertToVisual: (slug: string, id: string) =>
      request<ConvertToVisualResponse>(
        "POST",
        tp(slug, `/templates/${id}/convert-to-visual`),
      ),
    optOutVisual: (slug: string, id: string) =>
      request<TemplateView>(
        "POST",
        tp(slug, `/templates/${id}/opt-out-visual`),
      ),
  },
  renderPreview: (slug: string, body: RenderPreviewInput) =>
    request<RenderPreviewResponse>(
      "POST",
      tp(slug, "/render-preview"),
      { ...body },
    ),

  // ── Media library (Phase 6) ────────────────────────────────────────────────
  media: {
    list: (slug: string) =>
      request<{ items: Array<MediaAssetView> }>("GET", tp(slug, "/media")),
    upload: (slug: string, file: File) => {
      const form = new FormData()
      form.append("file", file)
      return requestMultipart<MediaUploadResult>(
        "POST",
        tp(slug, "/media"),
        form,
      )
    },
    remove: (slug: string, id: string) =>
      request("DELETE", tp(slug, `/media/${id}`)),
  },
}

export { ApiError }

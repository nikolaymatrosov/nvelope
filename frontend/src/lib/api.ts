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
  CampaignView,
  CreateCampaignInput,
  CreateListInput,
  CreateSubscriberInput,
  CreateTemplateInput,
  DomainView,
  InvitationLookup,
  IssuedAPIKey,
  JobStatusView,
  List,
  Membership,
  Node,
  Page,
  Permission,
  PlatformAccount,
  Role,
  SessionResult,
  StartExportInput,
  Subscriber,
  TOTPConfirmation,
  TOTPEnrolment,
  TemplateView,
  UpdateCampaignInput,
  UpdateListInput,
  UpdateSubscriberInput,
  UpdateTemplateInput,
  WorkspaceInfo,
  WorkspaceInvitation,
  WorkspaceSettings,
} from "./api-types"

const BASE: string = import.meta.env.VITE_API_BASE ?? ""

type Json = Record<string, unknown> | undefined

export type ApiResult<T = unknown> = { status: number; ok: boolean; data: T }

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
}

export { ApiError }

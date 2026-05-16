// Typed client for the nvelope platform and tenant APIs. Paths are relative so
// the browser sends the session cookie same-origin; the Vite dev server
// proxies /api and /t/{slug}/api to the Go API service.

const BASE: string = import.meta.env.VITE_API_BASE ?? ""

type Json = Record<string, unknown>

export type ApiResult<T = any> = { status: number; ok: boolean; data: T }

async function request<T = any>(
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
  let data: any = null
  const text = await res.text()
  if (text) data = JSON.parse(text)
  return { status: res.status, ok: res.ok, data }
}

export const api = {
  signup: (email: string, password: string, name: string) =>
    request("POST", "/api/platform/signup", { email, password, name }),
  login: (email: string, password: string) =>
    request("POST", "/api/platform/login", { email, password }),
  logout: () => request("POST", "/api/platform/logout"),
  me: () => request("GET", "/api/platform/me"),
  createTenant: (name: string, slug: string) =>
    request("POST", "/api/platform/tenants", { name, slug }),
  tenant: (slug: string) => request("GET", `/t/${slug}/api/tenant`),
  invite: (slug: string, email: string) =>
    request("POST", `/t/${slug}/api/invitations`, { email }),
  listInvitations: (slug: string) => request("GET", `/t/${slug}/api/invitations`),
  getInvitation: (token: string) =>
    request("GET", `/api/platform/invitations/${token}`),
  acceptInvitation: (token: string, password?: string, name?: string) =>
    request(
      "POST",
      `/api/platform/invitations/${token}/accept`,
      password !== undefined ? { password, name } : undefined,
    ),
}

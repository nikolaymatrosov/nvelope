// Vitest setup — polyfills jsdom lacks that shadcn primitives rely on, and
// initializes the i18next instance so components using useTranslation render
// real copy in tests.

import { vi } from "vitest"

import "@/i18n"

window.matchMedia = vi.fn().mockImplementation((query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addListener: vi.fn(),
  removeListener: vi.fn(),
  addEventListener: vi.fn(),
  removeEventListener: vi.fn(),
  dispatchEvent: vi.fn(),
}))

window.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// jsdom implements neither `fetch` nor `Request`, so Vitest leaves Node's
// (undici) versions in place — and undici's `Request` rejects a relative URL
// outright, where a browser resolves it against the document base. The
// generated tenant client (src/lib/tenant-client.ts) deliberately issues
// same-origin *relative* requests, and openapi-fetch builds a `Request` from
// them before it ever calls `fetch`. Give the environment the base origin the
// browser has instead of making production code construct absolute URLs it
// does not need.
const NodeRequest = globalThis.Request
globalThis.Request = class extends NodeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    super(
      typeof input === "string"
        ? new URL(input, window.location.href)
        : input,
      init,
    )
  }
}

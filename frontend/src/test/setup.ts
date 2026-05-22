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

// Shared test helper: render a component inside a fresh QueryClient with retry
// disabled so failed queries surface immediately.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render } from "@testing-library/react"
import type { ReactElement } from "react"

export function renderWithClient(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  const utils = render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  )
  return { client, ...utils }
}

import { afterEach, describe, expect, it, vi } from "vitest"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { renderHook, waitFor } from "@testing-library/react"
import { useJobStatus } from "./use-job-status"
import type { ReactNode } from "react"

import { api } from "@/lib/api"

vi.mock("@/lib/api", () => ({ api: { jobStatus: vi.fn() } }))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

const job = (status: string) => ({
  ID: "job-1",
  Kind: "import",
  Status: status,
  FileName: "people.csv",
  CreatedCount: 0,
  UpdatedCount: 0,
  FailedCount: 0,
  RowCount: 0,
  Failures: [],
})

afterEach(() => vi.clearAllMocks())

describe("useJobStatus", () => {
  it("does not query without a job id", () => {
    renderHook(() => useJobStatus("acme", null), { wrapper: wrapper() })
    expect(api.jobStatus).not.toHaveBeenCalled()
  })

  it("treats a running job as still polling", async () => {
    vi.mocked(api.jobStatus).mockResolvedValue(ok(job("running")))
    const { result } = renderHook(() => useJobStatus("acme", "job-1"), {
      wrapper: wrapper(),
    })
    await waitFor(() => expect(result.current.job).toBeDefined())
    expect(result.current.isTerminal).toBe(false)
    expect(result.current.isPolling).toBe(true)
  })

  it("treats a completed job as terminal", async () => {
    vi.mocked(api.jobStatus).mockResolvedValue(ok(job("completed")))
    const { result } = renderHook(() => useJobStatus("acme", "job-1"), {
      wrapper: wrapper(),
    })
    await waitFor(() => expect(result.current.job).toBeDefined())
    expect(result.current.isTerminal).toBe(true)
    expect(result.current.isPolling).toBe(false)
  })
})

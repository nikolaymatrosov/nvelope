import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { ExportPanel, ImportPanel } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => opts,
}))

vi.mock("@/lib/api", () => ({
  api: {
    listLists: vi.fn(),
    startImport: vi.fn(),
    startExport: vi.fn(),
    jobStatus: vi.fn(),
    downloadExportUrl: (slug: string, id: string) =>
      `/t/${slug}/api/jobs/${id}/download`,
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

const completedJob = {
  ID: "job-1",
  Kind: "import",
  Status: "completed",
  FileName: "people.csv",
  CreatedCount: 3,
  UpdatedCount: 1,
  FailedCount: 0,
  RowCount: 4,
  Failures: [],
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("ImportPanel", () => {
  it("uploads a file and shows the result summary", async () => {
    vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
    vi.mocked(api.startImport).mockResolvedValue(ok({ job_id: "job-1" }))
    vi.mocked(api.jobStatus).mockResolvedValue(ok(completedJob))
    renderWithClient(<ImportPanel slug="acme" />)

    const file = new File(["email\na@b.com"], "people.csv", {
      type: "text/csv",
    })
    fireEvent.change(await screen.findByLabelText(/csv or zip/i), {
      target: { files: [file] },
    })
    fireEvent.click(screen.getByRole("button", { name: /start import/i }))

    await waitFor(() =>
      expect(api.startImport).toHaveBeenCalledWith("acme", file, []),
    )
    expect(await screen.findByText(/3 created/i)).toBeDefined()
  })
})

describe("ExportPanel", () => {
  it("starts an export and offers a download when complete", async () => {
    vi.mocked(api.listLists).mockResolvedValue(ok({ lists: [], total: 0 }))
    vi.mocked(api.startExport).mockResolvedValue(ok({ job_id: "job-1" }))
    vi.mocked(api.jobStatus).mockResolvedValue(
      ok({ ...completedJob, Kind: "export" }),
    )
    renderWithClient(<ExportPanel slug="acme" />)

    fireEvent.click(await screen.findByRole("button", { name: /start export/i }))

    await waitFor(() =>
      expect(api.startExport).toHaveBeenCalledWith("acme", {
        selection: "all",
        list_id: undefined,
        segment: undefined,
      }),
    )
    const link = await screen.findByRole("link", { name: /download csv/i })
    expect(link.getAttribute("href")).toBe(
      "/t/acme/api/jobs/job-1/download",
    )
  })
})

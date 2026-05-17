import { createFileRoute } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { useMutation, useQuery } from "@tanstack/react-query"
import { DownloadIcon, UploadIcon } from "lucide-react"
import { toast } from "sonner"
import type { ExportSelection, JobStatusView, Node } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { useJobStatus } from "@/hooks/use-job-status"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Checkbox } from "@/components/ui/checkbox"
import { Progress } from "@/components/ui/progress"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { SegmentBuilder, emptyGroup } from "@/components/common/segment-builder"

export const Route = createFileRoute("/t/$slug/import-export/")({
  component: ImportExportView,
})

export function ImportExportView() {
  const { slug } = Route.useParams()
  const { can } = usePermissions(slug)
  const canImport = can("subscribers:import")
  const canExport = can("subscribers:export")

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">Import / Export</h1>
        <p className="text-sm text-muted-foreground">
          Bring subscribers in from a CSV, or export them out.
        </p>
      </header>

      <Tabs defaultValue="import">
        <TabsList>
          <TabsTrigger value="import">Import</TabsTrigger>
          <TabsTrigger value="export">Export</TabsTrigger>
        </TabsList>
        <TabsContent value="import" className="pt-4">
          {canImport ? (
            <ImportPanel slug={slug} />
          ) : (
            <PermissionNotice action="import subscribers" permission="subscribers:import" />
          )}
        </TabsContent>
        <TabsContent value="export" className="pt-4">
          {canExport ? (
            <ExportPanel slug={slug} />
          ) : (
            <PermissionNotice action="export subscribers" permission="subscribers:export" />
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}

function PermissionNotice({
  action,
  permission,
}: {
  action: string
  permission: string
}) {
  return (
    <Alert>
      <AlertTitle>Not available</AlertTitle>
      <AlertDescription>
        You need the “{permission}” permission to {action}. Ask a workspace
        admin to grant it.
      </AlertDescription>
    </Alert>
  )
}

function JobProgress({ job }: { job: JobStatusView }) {
  const done = job.CreatedCount + job.UpdatedCount + job.FailedCount
  const pct = job.RowCount > 0 ? Math.round((done / job.RowCount) * 100) : 0
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between text-sm">
        <span>Status</span>
        <Badge variant="secondary">{job.Status}</Badge>
      </div>
      {job.RowCount > 0 && <Progress value={pct} />}
    </div>
  )
}

function JobSummary({ job }: { job: JobStatusView }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        <Badge variant="secondary">{job.CreatedCount} created</Badge>
        <Badge variant="secondary">{job.UpdatedCount} updated</Badge>
        <Badge variant="secondary">{job.FailedCount} failed</Badge>
        <Badge variant="secondary">{job.RowCount} rows</Badge>
      </div>
      {job.Failures.length > 0 && (
        <div className="flex flex-col gap-1 rounded-lg border p-3">
          <p className="text-sm font-medium">Row failures</p>
          {job.Failures.map((f) => (
            <p key={f.Row} className="text-xs text-muted-foreground">
              Row {f.Row}: {f.Reason}
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Import ───────────────────────────────────────────────────────────────────

export function ImportPanel({ slug }: { slug: string }) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [listIds, setListIds] = useState<Array<string>>([])
  const [jobId, setJobId] = useState<string | null>(null)
  const { job, isPolling } = useJobStatus(slug, jobId)

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 100, offset: 0 })).data.lists,
  })

  const start = useMutation({
    mutationFn: () => api.startImport(slug, file as File, listIds),
    onSuccess: (res) => {
      setJobId(res.data.job_id)
      toast.success("Import started.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Import subscribers</CardTitle>
        <CardDescription>
          Upload a CSV or ZIP file and choose which lists to add subscribers to.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="import-file">CSV or ZIP file</Label>
          <input
            id="import-file"
            ref={fileRef}
            type="file"
            accept=".csv,.zip"
            className="text-sm"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label>Target lists</Label>
          {listsQuery.data && listsQuery.data.length > 0 ? (
            <div className="flex flex-col gap-2 rounded-lg border p-3">
              {listsQuery.data.map((list) => (
                <label
                  key={list.ID}
                  className="flex items-center gap-2 text-sm"
                >
                  <Checkbox
                    checked={listIds.includes(list.ID)}
                    onCheckedChange={(checked) =>
                      setListIds((prev) =>
                        checked
                          ? [...prev, list.ID]
                          : prev.filter((x) => x !== list.ID),
                      )
                    }
                  />
                  {list.Name}
                </label>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">No lists available.</p>
          )}
        </div>

        <div>
          <Button
            disabled={!file || start.isPending || isPolling}
            onClick={() => start.mutate()}
          >
            <UploadIcon /> Start import
          </Button>
        </div>

        {job && (
          <div className="flex flex-col gap-3">
            <JobProgress job={job} />
            {job.Status === "completed" && <JobSummary job={job} />}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ── Export ───────────────────────────────────────────────────────────────────

export function ExportPanel({ slug }: { slug: string }) {
  const [selection, setSelection] = useState<ExportSelection>("all")
  const [listId, setListId] = useState("")
  const [segment, setSegment] = useState<Node>(emptyGroup())
  const [jobId, setJobId] = useState<string | null>(null)
  const { job, isTerminal } = useJobStatus(slug, jobId)

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 100, offset: 0 })).data.lists,
  })

  const start = useMutation({
    mutationFn: () =>
      api.startExport(slug, {
        selection,
        list_id: selection === "list" ? listId : undefined,
        segment: selection === "segment" ? segment : undefined,
      }),
    onSuccess: (res) => {
      setJobId(res.data.job_id)
      toast.success("Export started.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Export subscribers</CardTitle>
        <CardDescription>
          Choose what to export. The result downloads as a CSV.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label>Selection</Label>
          <Select
            value={selection}
            onValueChange={(v) => setSelection(v as ExportSelection)}
          >
            <SelectTrigger className="w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">All subscribers</SelectItem>
                <SelectItem value="list">A list</SelectItem>
                <SelectItem value="segment">A segment</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        {selection === "list" && (
          <div className="flex flex-col gap-1.5">
            <Label>List</Label>
            <Select value={listId} onValueChange={setListId}>
              <SelectTrigger className="w-56">
                <SelectValue placeholder="Choose a list…" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {(listsQuery.data ?? []).map((l) => (
                    <SelectItem key={l.ID} value={l.ID}>
                      {l.Name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        )}

        {selection === "segment" && (
          <SegmentBuilder value={segment} onChange={setSegment} />
        )}

        <div>
          <Button
            disabled={
              start.isPending ||
              (selection === "list" && !listId) ||
              (jobId !== null && !isTerminal)
            }
            onClick={() => start.mutate()}
          >
            Start export
          </Button>
        </div>

        {job && (
          <div className="flex flex-col gap-3">
            <JobProgress job={job} />
            {job.Status === "completed" && (
              <Button asChild variant="outline">
                <a href={api.downloadExportUrl(slug, job.ID)}>
                  <DownloadIcon /> Download CSV
                </a>
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

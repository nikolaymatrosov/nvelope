import { createFileRoute } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { useMutation, useQuery } from "@tanstack/react-query"
import { DownloadIcon, UploadIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
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
  const { t } = useTranslation("importExport")
  const { can } = usePermissions(slug)
  const canImport = can("subscribers:import")
  const canExport = can("subscribers:export")

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("index.description")}
        </p>
      </header>

      <Tabs defaultValue="import">
        <TabsList>
          <TabsTrigger value="import">{t("tabs.import")}</TabsTrigger>
          <TabsTrigger value="export">{t("tabs.export")}</TabsTrigger>
        </TabsList>
        <TabsContent value="import" className="pt-4">
          {canImport ? (
            <ImportPanel slug={slug} />
          ) : (
            <PermissionNotice
              action={t("permissionNotice.importAction")}
              permission="subscribers:import"
            />
          )}
        </TabsContent>
        <TabsContent value="export" className="pt-4">
          {canExport ? (
            <ExportPanel slug={slug} />
          ) : (
            <PermissionNotice
              action={t("permissionNotice.exportAction")}
              permission="subscribers:export"
            />
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
  const { t } = useTranslation("importExport")
  return (
    <Alert>
      <AlertTitle>{t("permissionNotice.title")}</AlertTitle>
      <AlertDescription>
        {t("permissionNotice.description", { permission, action })}
      </AlertDescription>
    </Alert>
  )
}

function JobProgress({ job }: { job: JobStatusView }) {
  const { t } = useTranslation("importExport")
  const done = job.CreatedCount + job.UpdatedCount + job.FailedCount
  const pct = job.RowCount > 0 ? Math.round((done / job.RowCount) * 100) : 0
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between text-sm">
        <span>{t("job.status")}</span>
        <Badge variant="secondary">{job.Status}</Badge>
      </div>
      {job.RowCount > 0 && <Progress value={pct} />}
    </div>
  )
}

function JobSummary({ job }: { job: JobStatusView }) {
  const { t } = useTranslation("importExport")
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        <Badge variant="secondary">
          {t("job.createdCount", { count: job.CreatedCount })}
        </Badge>
        <Badge variant="secondary">
          {t("job.updatedCount", { count: job.UpdatedCount })}
        </Badge>
        <Badge variant="secondary">
          {t("job.failedCount", { count: job.FailedCount })}
        </Badge>
        <Badge variant="secondary">
          {t("job.rowCount", { count: job.RowCount })}
        </Badge>
      </div>
      {job.Failures.length > 0 && (
        <div className="flex flex-col gap-1 rounded-lg border p-3">
          <p className="text-sm font-medium">{t("job.rowFailures")}</p>
          {job.Failures.map((f) => (
            <p key={f.Row} className="text-xs text-muted-foreground">
              {t("job.rowFailure", { row: f.Row, reason: f.Reason })}
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Import ───────────────────────────────────────────────────────────────────

export function ImportPanel({ slug }: { slug: string }) {
  const { t } = useTranslation("importExport")
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
      toast.success(t("import.started"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("import.cardTitle")}</CardTitle>
        <CardDescription>{t("import.cardDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="import-file">{t("import.fileLabel")}</Label>
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
          <Label>{t("import.targetLists")}</Label>
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
            <p className="text-xs text-muted-foreground">
              {t("import.noLists")}
            </p>
          )}
        </div>

        <div>
          <Button
            disabled={!file || start.isPending || isPolling}
            onClick={() => start.mutate()}
          >
            <UploadIcon /> {t("import.start")}
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
  const { t } = useTranslation("importExport")
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
      toast.success(t("export.started"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("export.cardTitle")}</CardTitle>
        <CardDescription>{t("export.cardDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label>{t("export.selectionLabel")}</Label>
          <Select
            value={selection}
            onValueChange={(v) => setSelection(v as ExportSelection)}
          >
            <SelectTrigger className="w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">{t("export.selectionAll")}</SelectItem>
                <SelectItem value="list">
                  {t("export.selectionList")}
                </SelectItem>
                <SelectItem value="segment">
                  {t("export.selectionSegment")}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        {selection === "list" && (
          <div className="flex flex-col gap-1.5">
            <Label>{t("export.listLabel")}</Label>
            <Select value={listId} onValueChange={setListId}>
              <SelectTrigger className="w-56">
                <SelectValue placeholder={t("export.listPlaceholder")} />
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
            {t("export.start")}
          </Button>
        </div>

        {job && (
          <div className="flex flex-col gap-3">
            <JobProgress job={job} />
            {job.Status === "completed" && (
              <Button asChild variant="outline">
                <a href={api.downloadExportUrl(slug, job.ID)}>
                  <DownloadIcon /> {t("export.downloadCsv")}
                </a>
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

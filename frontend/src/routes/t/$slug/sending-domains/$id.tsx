import { Link, createFileRoute } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { RefreshCwIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import { StatusBadge } from "./index"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { formatDateTime } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { AsyncState } from "@/components/common/async-state"
import { DnsRecordRow } from "@/components/common/dns-record-row"

export const Route = createFileRoute("/t/$slug/sending-domains/$id")({
  component: SendingDomainDetail,
})

const POLL_INTERVAL_MS = 5000

export function SendingDomainDetail() {
  const { slug, id } = Route.useParams()
  const queryClient = useQueryClient()
  const { t } = useTranslation("sendingDomains")
  const { can } = usePermissions(slug)
  const canManage = can("sending:manage")

  const query = useQuery({
    queryKey: queryKeys.sendingDomain(slug, id),
    queryFn: async () => (await api.getSendingDomain(slug, id)).data,
    staleTime: 0,
    refetchInterval: (q) =>
      q.state.data?.status === "pending" ? POLL_INTERVAL_MS : false,
  })

  const recheck = useMutation({
    mutationFn: () => api.recheckSendingDomain(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.sendingDomain(slug, id),
      })
      await queryClient.invalidateQueries({
        queryKey: queryKeys.sendingDomains(slug),
      })
      toast.success(t("detail.recheckRequested"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/sending-domains"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          {t("detail.back")}
        </Link>
      </div>

      <AsyncState query={query}>
        {(domain) => (
          <>
            <header className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-semibold">{domain.domain}</h1>
                <StatusBadge status={domain.status} />
              </div>
              {canManage && domain.status !== "verified" && (
                <Button
                  variant="outline"
                  disabled={recheck.isPending}
                  onClick={() => recheck.mutate()}
                >
                  <RefreshCwIcon /> {t("detail.recheckNow")}
                </Button>
              )}
            </header>

            {domain.status === "failed" && domain.failure_reason && (
              <Alert variant="destructive">
                <AlertTitle>{t("detail.failedTitle")}</AlertTitle>
                <AlertDescription>{domain.failure_reason}</AlertDescription>
              </Alert>
            )}

            {domain.status === "verified" && (
              <Alert>
                <AlertTitle>{t("detail.verifiedTitle")}</AlertTitle>
                <AlertDescription>
                  {t("detail.verifiedMessage", {
                    date: formatDateTime(domain.verified_at),
                  })}
                </AlertDescription>
              </Alert>
            )}

            <Card>
              <CardHeader>
                <CardTitle>{t("detail.dnsTitle")}</CardTitle>
                <CardDescription>
                  {t("detail.dnsDescription")}
                  {domain.last_checked_at
                    ? t("detail.lastChecked", {
                        date: formatDateTime(domain.last_checked_at),
                      })
                    : ""}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                {domain.dkim_records.map((record, i) => (
                  <DnsRecordRow
                    key={`dkim-${i}`}
                    recordType={t("detail.dkimRecordType", {
                      type: record.type,
                    })}
                    host={record.name}
                    value={record.value}
                  />
                ))}
                <DnsRecordRow
                  recordType={t("detail.spfRecordType")}
                  host="@"
                  value={domain.spf_record}
                />
                <DnsRecordRow
                  recordType={t("detail.dmarcRecordType")}
                  host="_dmarc"
                  value={domain.dmarc_record}
                />
              </CardContent>
            </Card>
          </>
        )}
      </AsyncState>
    </div>
  )
}

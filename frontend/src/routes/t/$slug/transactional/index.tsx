import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/transactional/")({
  component: TransactionalView,
})

const PAYLOAD_EXAMPLE = `{
  "template_id": "<transactional template id>",
  "to": "recipient@example.com",
  "sending_domain_id": "<verified sending domain id>",
  "from_name": "Acme",
  "from_local_part": "no-reply",
  "variables": { "name": "Sam" }
}`

export function TransactionalView() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("transactional")
  const { can } = usePermissions(slug)
  const canManageKeys = can("apikeys:manage")

  const domainsQuery = useQuery({
    queryKey: queryKeys.sendingDomains(slug),
    queryFn: async () => (await api.listSendingDomains(slug)).data.domains,
  })
  const hasVerifiedDomain = (domainsQuery.data ?? []).some(
    (d) => d.status === "verified",
  )

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("index.description")}
        </p>
      </header>

      {domainsQuery.isSuccess && !hasVerifiedDomain && (
        <Alert>
          <AlertTitle>{t("domainAlert.title")}</AlertTitle>
          <AlertDescription>{t("domainAlert.description")}</AlertDescription>
        </Alert>
      )}

      <ApiKeysPanel slug={slug} canManageKeys={canManageKeys} />

      <Card>
        <CardHeader>
          <CardTitle>{t("endpoint.title")}</CardTitle>
          <CardDescription>{t("endpoint.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div className="flex flex-col gap-1">
            <span className="text-muted-foreground">
              {t("endpoint.endpointLabel")}
            </span>
            <code className="rounded bg-muted px-2 py-1 font-mono text-xs">
              POST /t/{slug}/api/tx
            </code>
          </div>
          <p className="text-muted-foreground">
            {t("endpoint.templateNotePrefix")}
            <strong>{t("endpoint.templateNoteEmphasis")}</strong>
            {t("endpoint.templateNoteSuffix")}
          </p>
          <div className="flex flex-col gap-1">
            <span className="text-muted-foreground">
              {t("endpoint.requestBodyLabel")}
            </span>
            <pre className="overflow-x-auto rounded bg-muted px-3 py-2 font-mono text-xs">
              {PAYLOAD_EXAMPLE}
            </pre>
          </div>
          <p className="text-muted-foreground">
            {t("endpoint.errorNotePrefix")}
            <code>403</code>
            {t("endpoint.errorNoteWith")}
            <code>quota_exceeded</code>
            {t("endpoint.errorNoteQuota")}
            <code>tenant_suspended</code>
            {t("endpoint.errorNoteSuspended")}
            <Link
              to="/t/$slug/billing"
              params={{ slug }}
              className="text-primary hover:underline"
            >
              {t("endpoint.errorNoteLink")}
            </Link>
            {t("endpoint.errorNoteEnd")}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function ApiKeysPanel({
  slug,
  canManageKeys,
}: {
  slug: string
  canManageKeys: boolean
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation("transactional")
  const [issueOpen, setIssueOpen] = useState(false)
  const [issuedToken, setIssuedToken] = useState<string | null>(null)
  const [revoking, setRevoking] = useState<string | null>(null)

  const keysQuery = useQuery({
    queryKey: queryKeys.apiKeys(slug),
    queryFn: async () => (await api.listAPIKeys(slug)).data.api_keys,
  })

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeAPIKey(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys(slug) })
      toast.success(t("apiKeys.revokeSuccess"))
      setRevoking(null)
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      setRevoking(null)
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("apiKeys.title")}</CardTitle>
        <CardDescription>{t("apiKeys.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {canManageKeys ? (
          <div>
            <Button onClick={() => setIssueOpen(true)}>
              <PlusIcon /> {t("apiKeys.issueKey")}
            </Button>
          </div>
        ) : (
          <Alert>
            <AlertTitle>{t("apiKeys.restrictedTitle")}</AlertTitle>
            <AlertDescription>
              {t("apiKeys.restrictedDescription")}
            </AlertDescription>
          </Alert>
        )}

        {issuedToken && (
          <Alert>
            <AlertTitle>{t("apiKeys.copyNowTitle")}</AlertTitle>
            <AlertDescription>
              <span className="flex flex-col gap-1 pt-1">
                <code className="rounded bg-muted px-2 py-1 font-mono text-xs">
                  {issuedToken}
                </code>
                <span className="text-xs">
                  {t("apiKeys.copyNowDescription")}
                </span>
              </span>
            </AlertDescription>
          </Alert>
        )}

        <AsyncState
          query={keysQuery}
          isEmpty={(d) => d.length === 0}
          emptyTitle={t("apiKeys.emptyTitle")}
          emptyMessage={t("apiKeys.emptyMessage")}
        >
          {(keys) => (
            <div className="flex flex-col gap-2">
              {keys.map((key) => (
                <div
                  key={key.ID}
                  className="flex items-center gap-3 rounded-lg border p-3"
                >
                  <div className="flex-1">
                    <p className="text-sm font-medium">{key.Name}</p>
                    <p className="text-xs text-muted-foreground">
                      {t("apiKeys.issued", {
                        date: formatDate(key.CreatedAt),
                      })}
                      {key.RevokedAt ? t("apiKeys.revokedSuffix") : ""}
                    </p>
                  </div>
                  {key.Permissions.includes("transactional:send") && (
                    <Badge variant="secondary">transactional:send</Badge>
                  )}
                  {canManageKeys && !key.RevokedAt && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t("apiKeys.revokeKey")}
                      onClick={() => setRevoking(key.ID)}
                    >
                      <Trash2Icon />
                    </Button>
                  )}
                </div>
              ))}
            </div>
          )}
        </AsyncState>
      </CardContent>

      <IssueKeyDialog
        slug={slug}
        open={issueOpen}
        onOpenChange={setIssueOpen}
        onIssued={(token) => setIssuedToken(token)}
      />

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={t("apiKeys.confirmRevokeTitle")}
        description={t("apiKeys.confirmRevokeDescription")}
        confirmLabel={t("apiKeys.confirmRevokeLabel")}
        busy={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking)}
      />
    </Card>
  )
}

function IssueKeyDialog({
  slug,
  open,
  onOpenChange,
  onIssued,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onIssued: (token: string) => void
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation(["transactional", "common"])

  const issue = useMutation({
    mutationFn: (name: string) =>
      api.issueAPIKey(slug, name.trim(), ["transactional:send"]),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys(slug) })
      onIssued(res.data.token)
      toast.success(t("issueDialog.issueSuccess"))
      form.reset()
      onOpenChange(false)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: "" },
    onSubmit: async ({ value }) => {
      await issue.mutateAsync(value.name).catch(() => {})
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("issueDialog.title")}</DialogTitle>
          <DialogDescription>
            {t("issueDialog.descriptionPrefix")}
            <code>transactional:send</code>
            {t("issueDialog.descriptionSuffix")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            form.handleSubmit()
          }}
        >
          <form.Field
            name="name"
            validators={{
              onBlur: compose(rules.required(t("issueDialog.nameRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("issueDialog.keyName")}
                required
                autoFocus
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common:actions.cancel")}
            </Button>
            <Button type="submit" disabled={issue.isPending}>
              {issue.isPending
                ? t("issueDialog.issuing")
                : t("issueDialog.issue")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

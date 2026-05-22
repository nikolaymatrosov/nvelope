import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { Permission, TOTPEnrolment } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { ALL_PERMISSIONS } from "@/lib/api-types"
import { formatDate } from "@/lib/format"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
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

export const Route = createFileRoute("/t/$slug/security/")({
  component: SecurityView,
})

export function SecurityView() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("security")
  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("index.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("index.description")}
        </p>
      </header>
      <Tabs defaultValue="totp">
        <TabsList>
          <TabsTrigger value="totp">{t("index.tabs.totp")}</TabsTrigger>
          <TabsTrigger value="apikeys">{t("index.tabs.apiKeys")}</TabsTrigger>
        </TabsList>
        <TabsContent value="totp" className="pt-4">
          <TotpPanel slug={slug} />
        </TabsContent>
        <TabsContent value="apikeys" className="pt-4">
          <ApiKeysPanel slug={slug} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ── TOTP enrolment ───────────────────────────────────────────────────────────

export function TotpPanel({ slug }: { slug: string }) {
  const { t } = useTranslation("security")
  const [enrolment, setEnrolment] = useState<TOTPEnrolment | null>(null)
  const [recoveryCodes, setRecoveryCodes] = useState<Array<string> | null>(null)
  const [confirmDisable, setConfirmDisable] = useState(false)

  const enable = useMutation({
    mutationFn: () => api.enableTOTP(slug),
    onSuccess: (res) => {
      setEnrolment(res.data)
      setRecoveryCodes(null)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const confirm = useMutation({
    mutationFn: (code: string) =>
      api.confirmTOTP(slug, enrolment?.secret ?? "", code.trim()),
    onSuccess: (res) => {
      setRecoveryCodes(res.data.recovery_codes)
      setEnrolment(null)
      toast.success(t("totp.enabledToast"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const disable = useMutation({
    mutationFn: () => api.disableTOTP(slug),
    onSuccess: () => {
      setEnrolment(null)
      setRecoveryCodes(null)
      toast.success(t("totp.disabledToast"))
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { code: "" },
    onSubmit: async ({ value }) => {
      await confirm.mutateAsync(value.code).catch(() => {})
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("totp.cardTitle")}</CardTitle>
        <CardDescription>
          {t("totp.cardDescription")}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {recoveryCodes && (
          <Alert>
            <AlertTitle>{t("totp.recoveryCodesTitle")}</AlertTitle>
            <AlertDescription>
              <span className="grid grid-cols-2 gap-1 pt-2 font-mono text-xs">
                {recoveryCodes.map((c) => (
                  <span key={c}>{c}</span>
                ))}
              </span>
            </AlertDescription>
          </Alert>
        )}

        {!enrolment && (
          <div className="flex gap-2">
            <Button
              disabled={enable.isPending}
              onClick={() => enable.mutate()}
            >
              {t("totp.enable")}
            </Button>
            <Button
              variant="outline"
              disabled={disable.isPending}
              onClick={() => setConfirmDisable(true)}
            >
              {t("totp.disable")}
            </Button>
          </div>
        )}

        {enrolment && (
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1">
              <p className="text-sm font-medium">
                {t("totp.secretLabel")}
              </p>
              <code className="rounded bg-muted px-2 py-1 font-mono text-sm">
                {enrolment.secret}
              </code>
              <a
                className="text-xs text-primary underline-offset-4 hover:underline"
                href={enrolment.uri}
              >
                {t("totp.openInApp")}
              </a>
            </div>
            <form
              className="flex items-end gap-2"
              noValidate
              onSubmit={(e) => {
                e.preventDefault()
                form.handleSubmit()
              }}
            >
              <div className="flex-1">
                <form.Field
                  name="code"
                  validators={{
                    onSubmit: compose(rules.required(t("totp.codeRequired"))),
                  }}
                >
                  {(field) => (
                    <FormField
                      label={t("totp.codeLabel")}
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      value={field.state.value}
                      onChange={(e) => field.handleChange(e.target.value)}
                      error={fieldError(field.state.meta.errors)}
                    />
                  )}
                </form.Field>
              </div>
              <Button type="submit" disabled={confirm.isPending}>
                {t("totp.confirm")}
              </Button>
            </form>
          </div>
        )}
      </CardContent>

      <ConfirmDialog
        open={confirmDisable}
        onOpenChange={setConfirmDisable}
        title={t("totp.confirmDisableTitle")}
        description={t("totp.confirmDisableDescription")}
        confirmLabel={t("totp.confirmDisableLabel")}
        busy={disable.isPending}
        onConfirm={() => {
          disable.mutate()
          setConfirmDisable(false)
        }}
      />
    </Card>
  )
}

// ── API keys ─────────────────────────────────────────────────────────────────

export function ApiKeysPanel({ slug }: { slug: string }) {
  const queryClient = useQueryClient()
  const { t } = useTranslation("security")
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
      toast.success(t("apiKeys.revokedToast"))
      setRevoking(null)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Button onClick={() => setIssueOpen(true)}>
          <PlusIcon /> {t("apiKeys.issue")}
        </Button>
      </div>

      {issuedToken && (
        <Alert>
          <AlertTitle>{t("apiKeys.issuedTitle")}</AlertTitle>
          <AlertDescription>
            <span className="flex flex-col gap-1 pt-1">
              <code className="rounded bg-muted px-2 py-1 font-mono text-xs">
                {issuedToken}
              </code>
              <span className="text-xs">
                {t("apiKeys.issuedNote")}
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
                    {t("apiKeys.issuedDate", {
                      date: formatDate(key.CreatedAt),
                    })}
                    {key.RevokedAt ? t("apiKeys.revokedSuffix") : ""}
                  </p>
                </div>
                <Badge variant="secondary">
                  {t("apiKeys.permissionCount", {
                    count: key.Permissions.length,
                  })}
                </Badge>
                {!key.RevokedAt && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t("apiKeys.revokeKeyAria")}
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
    </div>
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
  const { t } = useTranslation(["security", "common"])
  const [permissions, setPermissions] = useState<Array<Permission>>([])

  const issue = useMutation({
    mutationFn: (name: string) => api.issueAPIKey(slug, name.trim(), permissions),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys(slug) })
      onIssued(res.data.token)
      toast.success(t("issueDialog.issuedToast"))
      setPermissions([])
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
      <DialogContent className="max-h-[90svh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("issueDialog.title")}</DialogTitle>
          <DialogDescription>
            {t("issueDialog.description")}
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
                label={t("issueDialog.nameLabel")}
                required
                autoFocus
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <div className="flex flex-col gap-2">
            <p className="text-sm font-medium">
              {t("issueDialog.permissionsLabel")}
            </p>
            <div className="grid grid-cols-2 gap-2 rounded-lg border p-3">
              {ALL_PERMISSIONS.map((perm) => (
                <label key={perm} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={permissions.includes(perm)}
                    onCheckedChange={(checked) =>
                      setPermissions((prev) =>
                        checked
                          ? [...prev, perm]
                          : prev.filter((p) => p !== perm),
                      )
                    }
                  />
                  <code className="text-xs">{perm}</code>
                </label>
              ))}
            </div>
          </div>
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
                ? t("issueDialog.submitting")
                : t("issueDialog.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

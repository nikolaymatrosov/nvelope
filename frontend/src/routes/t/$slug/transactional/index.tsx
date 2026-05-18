import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
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
        <h1 className="text-2xl font-semibold">Transactional sending</h1>
        <p className="text-sm text-muted-foreground">
          Issue a scoped API key and integrate the transactional endpoint.
        </p>
      </header>

      {domainsQuery.isSuccess && !hasVerifiedDomain && (
        <Alert>
          <AlertTitle>A verified sending domain is required</AlertTitle>
          <AlertDescription>
            Transactional sends will fail until at least one sending domain is
            verified. Verify a domain under Sending Domains first.
          </AlertDescription>
        </Alert>
      )}

      <ApiKeysPanel slug={slug} canManageKeys={canManageKeys} />

      <Card>
        <CardHeader>
          <CardTitle>Endpoint reference</CardTitle>
          <CardDescription>
            Send transactional mail by calling this endpoint with the API key
            above as a Bearer token.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div className="flex flex-col gap-1">
            <span className="text-muted-foreground">Endpoint</span>
            <code className="rounded bg-muted px-2 py-1 font-mono text-xs">
              POST /t/{slug}/api/tx
            </code>
          </div>
          <p className="text-muted-foreground">
            Each request must reference an existing{" "}
            <strong>transactional</strong> template by id and a verified
            sending domain id.
          </p>
          <div className="flex flex-col gap-1">
            <span className="text-muted-foreground">Request body</span>
            <pre className="overflow-x-auto rounded bg-muted px-3 py-2 font-mono text-xs">
              {PAYLOAD_EXAMPLE}
            </pre>
          </div>
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
      toast.success("API key revoked.")
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
        <CardTitle>API keys</CardTitle>
        <CardDescription>
          Keys scoped for transactional sending.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {canManageKeys ? (
          <div>
            <Button onClick={() => setIssueOpen(true)}>
              <PlusIcon /> Issue API key
            </Button>
          </div>
        ) : (
          <Alert>
            <AlertTitle>Key management is restricted</AlertTitle>
            <AlertDescription>
              You do not have permission to issue or revoke API keys in this
              workspace.
            </AlertDescription>
          </Alert>
        )}

        {issuedToken && (
          <Alert>
            <AlertTitle>Copy your API key now</AlertTitle>
            <AlertDescription>
              <span className="flex flex-col gap-1 pt-1">
                <code className="rounded bg-muted px-2 py-1 font-mono text-xs">
                  {issuedToken}
                </code>
                <span className="text-xs">
                  This secret is shown only once and cannot be retrieved later.
                  If you lose it, revoke the key and issue a new one.
                </span>
              </span>
            </AlertDescription>
          </Alert>
        )}

        <AsyncState
          query={keysQuery}
          isEmpty={(d) => d.length === 0}
          emptyTitle="No API keys"
          emptyMessage="Issue an API key to integrate transactional sending."
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
                      Issued {formatDate(key.CreatedAt)}
                      {key.RevokedAt ? " · revoked" : ""}
                    </p>
                  </div>
                  {key.Permissions.includes("transactional:send") && (
                    <Badge variant="secondary">transactional:send</Badge>
                  )}
                  {canManageKeys && !key.RevokedAt && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Revoke key"
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
        title="Revoke this API key?"
        description="Any integration using this key will immediately stop working."
        confirmLabel="Revoke key"
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

  const issue = useMutation({
    mutationFn: (name: string) =>
      api.issueAPIKey(slug, name.trim(), ["transactional:send"]),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys(slug) })
      onIssued(res.data.token)
      toast.success("API key issued.")
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
          <DialogTitle>Issue a transactional API key</DialogTitle>
          <DialogDescription>
            The key is scoped to <code>transactional:send</code>. Its secret is
            shown once after it is created.
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
            validators={{ onBlur: compose(rules.required("Name the key.")) }}
          >
            {(field) => (
              <FormField
                label="Key name"
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
              Cancel
            </Button>
            <Button type="submit" disabled={issue.isPending}>
              {issue.isPending ? "Issuing…" : "Issue key"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

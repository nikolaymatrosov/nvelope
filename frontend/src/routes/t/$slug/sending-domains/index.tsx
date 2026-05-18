import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, RefreshCwIcon } from "lucide-react"
import { toast } from "sonner"
import type { DomainStatus } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { formatDate } from "@/lib/format"
import { usePermissions } from "@/hooks/use-permissions"
import { useSendingDomains } from "@/hooks/use-sending-domains"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AsyncState } from "@/components/common/async-state"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/sending-domains/")({
  component: SendingDomainsView,
})

const STATUS_VARIANT: Record<
  DomainStatus,
  "default" | "secondary" | "destructive"
> = {
  verified: "default",
  pending: "secondary",
  failed: "destructive",
}

export function StatusBadge({ status }: { status: DomainStatus }) {
  return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>
}

export function SendingDomainsView() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)
  const canManage = can("sending:manage")
  const [createOpen, setCreateOpen] = useState(false)

  const { query } = useSendingDomains(slug)

  const recheck = useMutation({
    mutationFn: (id: string) => api.recheckSendingDomain(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.sendingDomains(slug),
      })
      toast.success("Re-check requested.")
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Sending domains</h1>
          <p className="text-sm text-muted-foreground">
            Verify the domains you send mail from.
          </p>
        </div>
        {canManage && (
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon /> Add domain
          </Button>
        )}
      </header>

      <AsyncState
        query={query}
        isEmpty={(d) => d.length === 0}
        emptyTitle="No sending domains yet"
        emptyMessage="Add a domain to publish its DNS records and start sending."
        emptyAction={
          canManage ? (
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon /> Add domain
            </Button>
          ) : undefined
        }
      >
        {(domains) => (
          <div className="flex flex-col gap-2">
            {domains.map((domain) => (
              <div
                key={domain.id}
                className="flex cursor-pointer items-center gap-3 rounded-lg border p-3 hover:bg-muted/50"
                onClick={() =>
                  navigate({
                    to: "/t/$slug/sending-domains/$id",
                    params: { slug, id: domain.id },
                  })
                }
              >
                <div className="flex-1">
                  <p className="text-sm font-medium">{domain.domain}</p>
                  <p className="text-xs text-muted-foreground">
                    Added {formatDate(domain.created_at)}
                  </p>
                </div>
                <StatusBadge status={domain.status} />
                {canManage && domain.status !== "verified" && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Re-check domain"
                    disabled={recheck.isPending}
                    onClick={(e) => {
                      e.stopPropagation()
                      recheck.mutate(domain.id)
                    }}
                  >
                    <RefreshCwIcon />
                  </Button>
                )}
              </div>
            ))}
          </div>
        )}
      </AsyncState>

      <AddDomainDialog
        slug={slug}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
    </div>
  )
}

function AddDomainDialog({
  slug,
  open,
  onOpenChange,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const create = useMutation({
    mutationFn: (domain: string) => api.addSendingDomain(slug, domain.trim()),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.sendingDomains(slug),
      })
      toast.success("Domain added. Publish its DNS records to verify it.")
      onOpenChange(false)
      form.reset()
      navigate({
        to: "/t/$slug/sending-domains/$id",
        params: { slug, id: res.data.id },
      })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { domain: "" },
    onSubmit: async ({ value }) => {
      await create.mutateAsync(value.domain).catch(() => {})
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add a sending domain</DialogTitle>
          <DialogDescription>
            Enter the domain you want to send mail from. You will be shown the
            DNS records to publish.
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
            name="domain"
            validators={{
              onBlur: compose(rules.required("Enter a domain name.")),
            }}
          >
            {(field) => (
              <FormField
                label="Domain"
                required
                autoFocus
                placeholder="mail.example.com"
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
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Adding…" : "Add domain"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

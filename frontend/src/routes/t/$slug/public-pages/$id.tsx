// Public pages — create / edit one subscription page (US4). Reuses the
// listSubscriptionPages query to populate the form for an existing page (the
// backend exposes no GET /subscription-pages/{id} endpoint, only a list); the
// special id `"new"` opens an empty form for creation.

import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CopyIcon, ExternalLinkIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { subscriptionPageUrl } from "./index"
import type {
  SaveSubscriptionPageInput,
  SubscriptionPageFieldView,
  SubscriptionPageView,
} from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage, isNotFound } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { AsyncState } from "@/components/common/async-state"
import {
  FormField,
  compose,
  fieldError,
  rules,
} from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/public-pages/$id")({
  component: SubscriptionPageEdit,
})

type FormValues = {
  slug: string
  title: string
  target_list_ids: Array<string>
  fields: Array<SubscriptionPageFieldView>
  sending_domain_id: string
  from_name: string
  from_local_part: string
  active: boolean
}

const EMPTY_VALUES: FormValues = {
  slug: "",
  title: "",
  target_list_ids: [],
  fields: [],
  sending_domain_id: "",
  from_name: "",
  from_local_part: "",
  active: true,
}

function toFormValues(page: SubscriptionPageView): FormValues {
  return {
    slug: page.Slug,
    title: page.Title,
    target_list_ids: page.TargetListIDs,
    fields: page.Fields,
    sending_domain_id: page.SendingDomainID,
    from_name: page.FromName,
    from_local_part: page.FromLocalPart,
    active: page.Active,
  }
}

export function SubscriptionPageEdit() {
  const { slug, id } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions(slug)
  const canManage = can("subscription_pages:manage")
  const isNew = id === "new"

  const pagesQuery = useQuery({
    queryKey: queryKeys.subscriptionPages(slug),
    queryFn: async () =>
      (await api.subscriptionPages.list(slug)).data.subscription_pages,
    enabled: !isNew,
  })

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () => (await api.listLists(slug)).data.lists,
  })

  const sendingDomainsQuery = useQuery({
    queryKey: queryKeys.sendingDomains(slug),
    queryFn: async () => (await api.listSendingDomains(slug)).data.domains,
  })

  const existing = !isNew
    ? pagesQuery.data?.find((p) => p.ID === id)
    : undefined

  if (!canManage) {
    return (
      <Empty data-testid="public-pages-forbidden" className="border">
        <EmptyHeader>
          <EmptyTitle>You do not have access</EmptyTitle>
          <EmptyDescription>
            You need the subscription_pages:manage permission to view this
            page.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  if (!isNew && pagesQuery.isLoading) {
    return <AsyncState query={pagesQuery}>{() => null}</AsyncState>
  }

  if (!isNew && pagesQuery.isError) {
    return <AsyncState query={pagesQuery}>{() => null}</AsyncState>
  }

  if (!isNew && pagesQuery.data && !existing) {
    return (
      <Empty data-testid="public-page-not-found" className="border">
        <EmptyHeader>
          <EmptyTitle>Subscription page not found</EmptyTitle>
          <EmptyDescription>
            This page may have been deleted, or you may have followed an old
            link.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <SubscriptionPageForm
      slug={slug}
      pageId={isNew ? undefined : id}
      initial={existing ? toFormValues(existing) : EMPTY_VALUES}
      onSaved={async (saved) => {
        await queryClient.invalidateQueries({
          queryKey: queryKeys.subscriptionPages(slug),
        })
        await queryClient.invalidateQueries({
          queryKey: queryKeys.subscriptionPage(slug, saved.ID),
        })
        if (isNew) {
          await navigate({
            to: "/t/$slug/public-pages/$id",
            params: { slug, id: saved.ID },
          })
        }
      }}
      listsLoading={listsQuery.isLoading}
      lists={listsQuery.data ?? []}
      domainsLoading={sendingDomainsQuery.isLoading}
      domains={sendingDomainsQuery.data ?? []}
    />
  )
}

type SubscriptionPageFormProps = {
  slug: string
  pageId?: string
  initial: FormValues
  onSaved: (saved: SubscriptionPageView) => Promise<void> | void
  listsLoading: boolean
  lists: Array<{ ID: string; Name: string }>
  domainsLoading: boolean
  domains: Array<{ id: string; domain: string; status: string }>
}

function SubscriptionPageForm({
  slug,
  pageId,
  initial,
  onSaved,
  listsLoading,
  lists,
  domainsLoading,
  domains,
}: SubscriptionPageFormProps) {
  const [serverError, setServerError] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: async (input: SaveSubscriptionPageInput) => {
      if (pageId) {
        return (await api.subscriptionPages.update(slug, pageId, input)).data
      }
      return (await api.subscriptionPages.create(slug, input)).data
    },
    onSuccess: async (saved) => {
      setServerError(null)
      toast.success(pageId ? "Subscription page updated." : "Subscription page created.")
      await onSaved(saved)
    },
    onError: (e) => {
      const msg = errorMessage(e)
      setServerError(msg)
      if (!isNotFound(e)) toast.error(msg)
    },
  })

  const form = useForm({
    defaultValues: initial,
    onSubmit: async ({ value }) => {
      if (value.target_list_ids.length === 0) {
        setServerError("Select at least one list to subscribe visitors to.")
        return
      }
      await save.mutateAsync({
        slug: value.slug.trim(),
        title: value.title.trim(),
        target_list_ids: value.target_list_ids,
        fields: value.fields.map((f) => ({
          key: f.key.trim(),
          label: f.label.trim(),
          required: f.required,
        })),
        sending_domain_id: value.sending_domain_id,
        from_name: value.from_name.trim(),
        from_local_part: value.from_local_part.trim(),
        active: value.active,
      })
    },
  })

  useEffect(() => {
    form.reset(initial)
  }, [form, initial, pageId])

  const publicUrl = pageId && initial.slug ? subscriptionPageUrl(slug, initial.slug) : null

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/public-pages"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          ← Public pages
        </Link>
      </div>

      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          {pageId ? "Edit subscription page" : "New subscription page"}
        </h1>
        {publicUrl && (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                navigator.clipboard.writeText(publicUrl).then(
                  () => toast.success("Copied"),
                  () => toast.error("Could not copy"),
                )
              }}
              data-testid="copy-public-url"
            >
              <CopyIcon /> Copy URL
            </Button>
            <Button variant="ghost" size="sm" asChild data-testid="preview-public-url">
              <a href={publicUrl} target="_blank" rel="noreferrer">
                <ExternalLinkIcon /> Preview
              </a>
            </Button>
          </div>
        )}
      </header>

      <form
        onSubmit={(e) => {
          e.preventDefault()
          form.handleSubmit()
        }}
        className="flex flex-col gap-6"
      >
        {serverError && (
          <p className="text-sm text-destructive" role="alert" data-testid="form-server-error">
            {serverError}
          </p>
        )}

        <Card>
          <CardHeader>
            <CardTitle>Basics</CardTitle>
            <CardDescription>
              The page's identity on the public URL and what visitors see.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <form.Field
              name="slug"
              validators={{
                onChange: compose(rules.required(), rules.slug()),
              }}
            >
              {(field) => (
                <FormField
                  label="Slug"
                  required
                  hint="Lowercase letters, numbers, and hyphens. Appears in the public URL."
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field name="title" validators={{ onChange: compose(rules.required()) }}>
              {(field) => (
                <FormField
                  label="Title"
                  required
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field name="active">
              {(field) => (
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={field.state.value}
                    onCheckedChange={(c) => field.handleChange(Boolean(c))}
                    data-testid="page-active"
                  />
                  Active — visible to visitors at the public URL
                </label>
              )}
            </form.Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Bound lists</CardTitle>
            <CardDescription>
              A confirmed subscriber will be added to every list selected here.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {listsLoading ? (
              <p className="text-sm text-muted-foreground">Loading lists…</p>
            ) : lists.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No lists yet. Create a list first.
              </p>
            ) : (
              <form.Field name="target_list_ids">
                {(field) => (
                  <ul className="flex flex-col gap-2" data-testid="bound-lists">
                    {lists.map((list) => {
                      const checked = field.state.value.includes(list.ID)
                      return (
                        <li key={list.ID}>
                          <label className="flex items-center gap-2 text-sm">
                            <Checkbox
                              checked={checked}
                              onCheckedChange={(c) => {
                                if (c) {
                                  field.handleChange([
                                    ...field.state.value,
                                    list.ID,
                                  ])
                                } else {
                                  field.handleChange(
                                    field.state.value.filter((x) => x !== list.ID),
                                  )
                                }
                              }}
                            />
                            {list.Name}
                          </label>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </form.Field>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Fields</CardTitle>
            <CardDescription>
              Email is always shown and always required. Add custom fields the
              visitor must (or may) fill in.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form.Field name="fields">
              {(field) => (
                <div className="flex flex-col gap-3" data-testid="fields-editor">
                  {field.state.value.map((row, idx) => (
                    <div
                      key={idx}
                      className="grid grid-cols-1 gap-2 rounded-md border p-3 sm:grid-cols-[1fr_1fr_auto_auto]"
                    >
                      <div>
                        <Label htmlFor={`field-key-${idx}`}>Key</Label>
                        <Input
                          id={`field-key-${idx}`}
                          value={row.key}
                          onChange={(e) => {
                            const next = [...field.state.value]
                            next[idx] = { ...row, key: e.target.value }
                            field.handleChange(next)
                          }}
                        />
                      </div>
                      <div>
                        <Label htmlFor={`field-label-${idx}`}>Label</Label>
                        <Input
                          id={`field-label-${idx}`}
                          value={row.label}
                          onChange={(e) => {
                            const next = [...field.state.value]
                            next[idx] = { ...row, label: e.target.value }
                            field.handleChange(next)
                          }}
                        />
                      </div>
                      <label className="flex items-center gap-2 self-end text-sm">
                        <Checkbox
                          checked={row.required}
                          onCheckedChange={(c) => {
                            const next = [...field.state.value]
                            next[idx] = { ...row, required: Boolean(c) }
                            field.handleChange(next)
                          }}
                        />
                        Required
                      </label>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="self-end"
                        onClick={() =>
                          field.handleChange(
                            field.state.value.filter((_, i) => i !== idx),
                          )
                        }
                        aria-label={`Remove field ${row.label || row.key || idx + 1}`}
                      >
                        <Trash2Icon />
                      </Button>
                    </div>
                  ))}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      field.handleChange([
                        ...field.state.value,
                        { key: "", label: "", required: false },
                      ])
                    }
                    data-testid="add-field"
                  >
                    <PlusIcon /> Add field
                  </Button>
                </div>
              )}
            </form.Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Confirmation email</CardTitle>
            <CardDescription>
              The double-opt-in confirmation email is sent from this verified
              sending domain.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <form.Field name="sending_domain_id" validators={{ onChange: compose(rules.required()) }}>
              {(field) => (
                <FormField
                  label="Sending domain"
                  required
                  error={fieldError(field.state.meta.errors)}
                >
                  <select
                    id="sending-domain"
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    className="h-10 rounded-md border px-3 text-sm"
                    data-testid="sending-domain-select"
                    disabled={domainsLoading}
                  >
                    <option value="">Select a sending domain…</option>
                    {domains.map((d) => (
                      <option key={d.id} value={d.id}>
                        {d.domain} {d.status !== "verified" ? `(${d.status})` : ""}
                      </option>
                    ))}
                  </select>
                </FormField>
              )}
            </form.Field>
            <form.Field name="from_name" validators={{ onChange: compose(rules.required()) }}>
              {(field) => (
                <FormField
                  label="From name"
                  required
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field name="from_local_part" validators={{ onChange: compose(rules.required()) }}>
              {(field) => (
                <FormField
                  label="From address (local part)"
                  required
                  hint="The portion before the @ in the From address."
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
          </CardContent>
        </Card>

        <div className="flex items-center justify-end gap-2">
          <Button type="submit" disabled={save.isPending} data-testid="save-page">
            {save.isPending ? "Saving…" : pageId ? "Save changes" : "Create page"}
          </Button>
        </div>
      </form>
    </div>
  )
}

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
import { useTranslation } from "react-i18next"
import { subscriptionPageUrl } from "./index"
import type {
  Field,
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
  const { t } = useTranslation("publicPages")
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

  // FR-016b: the subscription-page "visible profile fields" picker reads
  // from the same subscriber-fields registry the visual editor's merge-tag
  // picker uses. Built-in pseudo-rows + tenant custom fields, one source.
  const subscriberFieldsQuery = useQuery({
    queryKey: queryKeys.subscriberFields(slug),
    queryFn: async () => (await api.subscriberFields.list(slug)).data.fields,
  })

  const existing = !isNew
    ? pagesQuery.data?.find((p) => p.ID === id)
    : undefined

  if (!canManage) {
    return (
      <Empty data-testid="public-pages-forbidden" className="border">
        <EmptyHeader>
          <EmptyTitle>{t("forbidden.title")}</EmptyTitle>
          <EmptyDescription>{t("forbidden.description")}</EmptyDescription>
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
          <EmptyTitle>{t("notFound.title")}</EmptyTitle>
          <EmptyDescription>{t("notFound.description")}</EmptyDescription>
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
      fieldsLoading={subscriberFieldsQuery.isLoading}
      fields={subscriberFieldsQuery.data ?? []}
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
  fieldsLoading: boolean
  fields: Array<Field>
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
  fieldsLoading,
  fields,
}: SubscriptionPageFormProps) {
  const { t } = useTranslation("publicPages")
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
      toast.success(
        pageId ? t("edit.updateSuccess") : t("edit.createSuccess"),
      )
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
        setServerError(t("edit.selectListError"))
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
          {t("edit.back")}
        </Link>
      </div>

      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          {pageId ? t("edit.titleEdit") : t("edit.titleNew")}
        </h1>
        {publicUrl && (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                navigator.clipboard.writeText(publicUrl).then(
                  () => toast.success(t("edit.copied")),
                  () => toast.error(t("edit.copyFailed")),
                )
              }}
              data-testid="copy-public-url"
            >
              <CopyIcon /> {t("edit.copyUrl")}
            </Button>
            <Button variant="ghost" size="sm" asChild data-testid="preview-public-url">
              <a href={publicUrl} target="_blank" rel="noreferrer">
                <ExternalLinkIcon /> {t("edit.preview")}
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
            <CardTitle>{t("basics.title")}</CardTitle>
            <CardDescription>{t("basics.description")}</CardDescription>
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
                  label={t("basics.slug")}
                  required
                  hint={t("basics.slugHint")}
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field name="title" validators={{ onChange: compose(rules.required()) }}>
              {(field) => (
                <FormField
                  label={t("basics.pageTitle")}
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
                  {t("basics.active")}
                </label>
              )}
            </form.Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("boundLists.title")}</CardTitle>
            <CardDescription>{t("boundLists.description")}</CardDescription>
          </CardHeader>
          <CardContent>
            {listsLoading ? (
              <p className="text-sm text-muted-foreground">
                {t("boundLists.loading")}
              </p>
            ) : lists.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t("boundLists.empty")}
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
            <CardTitle>{t("fields.title")}</CardTitle>
            <CardDescription>{t("fields.description")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form.Field name="fields">
              {(field) => {
                const chosen = new Set(field.state.value.map((f) => f.key))
                const available = fields.filter(
                  (f) => f.slug !== "email" && !chosen.has(f.slug),
                )
                const fieldBySlug = new Map(fields.map((f) => [f.slug, f]))
                return (
                  <div className="flex flex-col gap-3" data-testid="fields-editor">
                    {field.state.value.map((row, idx) => {
                      const meta = fieldBySlug.get(row.key)
                      // Selectable options for this row: every registry entry
                      // not chosen on another row, plus the row's own slug
                      // (so the current pick stays visible while editing).
                      const options = fields.filter(
                        (f) =>
                          f.slug !== "email" &&
                          (f.slug === row.key || !chosen.has(f.slug)),
                      )
                      return (
                        <div
                          key={idx}
                          className="grid grid-cols-1 gap-2 rounded-md border p-3 sm:grid-cols-[1fr_1fr_auto_auto]"
                        >
                          <div>
                            <Label htmlFor={`field-key-${idx}`}>
                              {t("fields.fieldLabel")}
                            </Label>
                            <select
                              id={`field-key-${idx}`}
                              value={row.key}
                              onChange={(e) => {
                                const pickedSlug = e.target.value
                                const picked = fieldBySlug.get(pickedSlug)
                                const next = [...field.state.value]
                                next[idx] = {
                                  ...row,
                                  key: pickedSlug,
                                  // Auto-fill the label from the registry the
                                  // first time the operator picks a slug.
                                  // Manual edits afterwards survive a re-pick
                                  // because we only overwrite an empty label
                                  // or one that matched the previously-chosen
                                  // field's displayName.
                                  label:
                                    !row.label ||
                                    (meta && row.label === meta.displayName)
                                      ? picked
                                        ? picked.displayName
                                        : ""
                                      : row.label,
                                }
                                field.handleChange(next)
                              }}
                              className="h-10 w-full rounded-md border px-3 text-sm"
                              data-testid={`field-key-${idx}`}
                              disabled={fieldsLoading}
                            >
                              <option value="">
                                {fieldsLoading
                                  ? t("fields.loadingOption")
                                  : t("fields.chooseOption")}
                              </option>
                              {options.map((f) => (
                                <option key={f.slug} value={f.slug}>
                                  {f.displayName}
                                  {f.builtIn ? t("fields.builtInSuffix") : ""}
                                </option>
                              ))}
                              {row.key && !fieldBySlug.has(row.key) && (
                                // A persisted page may reference a slug whose
                                // registry entry was deleted later. Keep it
                                // visible so the operator can re-pick rather
                                // than silently losing it on save.
                                <option value={row.key}>
                                  {t("fields.deletedSuffix", { key: row.key })}
                                </option>
                              )}
                            </select>
                          </div>
                          <div>
                            <Label htmlFor={`field-label-${idx}`}>
                              {t("fields.labelLabel")}
                            </Label>
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
                            {t("fields.required")}
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
                            aria-label={t("fields.removeField", {
                              name: row.label || row.key || idx + 1,
                            })}
                          >
                            <Trash2Icon />
                          </Button>
                        </div>
                      )
                    })}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        // The button is disabled when no unused options
                        // remain, so available[0] is always defined here.
                        const next = available[0]
                        field.handleChange([
                          ...field.state.value,
                          {
                            key: next.slug,
                            label: next.displayName,
                            required: false,
                          },
                        ])
                      }}
                      data-testid="add-field"
                      disabled={fieldsLoading || available.length === 0}
                    >
                      <PlusIcon /> {t("fields.addField")}
                    </Button>
                    {!fieldsLoading && fields.length <= 1 && (
                      <p className="text-xs text-muted-foreground">
                        {t("fields.noCustomFields")}
                      </p>
                    )}
                  </div>
                )
              }}
            </form.Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("confirmationEmail.title")}</CardTitle>
            <CardDescription>
              {t("confirmationEmail.description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <form.Field name="sending_domain_id" validators={{ onChange: compose(rules.required()) }}>
              {(field) => (
                <FormField
                  label={t("confirmationEmail.sendingDomain")}
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
                    <option value="">
                      {t("confirmationEmail.selectDomain")}
                    </option>
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
                  label={t("confirmationEmail.fromName")}
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
                  label={t("confirmationEmail.fromLocalPart")}
                  required
                  hint={t("confirmationEmail.fromLocalPartHint")}
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
            {save.isPending
              ? t("edit.saving")
              : pageId
                ? t("edit.save")
                : t("edit.create")}
          </Button>
        </div>
      </form>
    </div>
  )
}

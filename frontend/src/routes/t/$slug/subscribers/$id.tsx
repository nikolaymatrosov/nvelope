import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon, XIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import type { Subscriber, SubscriptionStatus } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
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
import { AsyncState } from "@/components/common/async-state"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"
import { JsonAttributeEditor } from "@/components/common/json-attribute-editor"

export const Route = createFileRoute("/t/$slug/subscribers/$id")({
  component: SubscriberDetail,
})

// Annotated with the generated enums rather than inferred as Array<string>:
// that makes a value the server would reject a compile error here, at the
// point it is written down. ("blocklisted" is a third SubscriberState, but it
// is set by the bounce pipeline, not by hand.)
const STATES: Array<Subscriber["state"]> = ["enabled", "disabled"]
// The real domain values (there is no "subscribed" or "pending" status
// server-side): unconfirmed (awaiting confirmation), confirmed, unsubscribed.
const SUB_STATES: Array<SubscriptionStatus> = [
  "unconfirmed",
  "confirmed",
  "unsubscribed",
]

// Radix hands `onValueChange` a bare string. Narrow it against the option list
// that produced it instead of asserting with `as`: the assertion would be a
// promise TypeScript cannot check, and an unchecked one is how the hardcoded
// "subscribed" status — which the Go validator always rejected — survived.
function optionOf<T extends string>(
  options: Array<T>,
  value: string,
): T | undefined {
  return options.find((option) => option === value)
}

export function SubscriberDetail() {
  const { slug, id } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { t } = useTranslation("subscribers")
  const [confirmOpen, setConfirmOpen] = useState(false)

  const subscriberQuery = useQuery({
    queryKey: queryKeys.subscriber(slug, id),
    queryFn: async () => (await api.getSubscriber(slug, id)).data,
  })

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 100, offset: 0 })).data.items,
  })

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: queryKeys.subscriber(slug, id) })
    queryClient.invalidateQueries({ queryKey: queryKeys.subscribers(slug) })
  }

  const remove = useMutation({
    mutationFn: () => api.deleteSubscriber(slug, id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.subscribers(slug),
      })
      toast.success(t("detail.deleteSuccess"))
      navigate({ to: "/t/$slug/subscribers", params: { slug } })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          to="/t/$slug/subscribers"
          params={{ slug }}
          className="text-sm text-muted-foreground hover:underline"
        >
          {t("detail.back")}
        </Link>
      </div>

      <AsyncState query={subscriberQuery}>
        {(subscriber) => (
          <div className="flex flex-col gap-6">
            <EditSubscriberCard
              key={subscriber.id}
              slug={slug}
              subscriber={subscriber}
              onSaved={invalidate}
              onDelete={() => setConfirmOpen(true)}
            />
            <MembershipsCard
              slug={slug}
              subscriber={subscriber}
              lists={listsQuery.data ?? []}
              onChanged={invalidate}
            />
          </div>
        )}
      </AsyncState>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("detail.deleteTitle")}
        description={t("detail.deleteDescription")}
        confirmLabel={t("detail.deleteConfirm")}
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </div>
  )
}

function EditSubscriberCard({
  slug,
  subscriber,
  onSaved,
  onDelete,
}: {
  slug: string
  subscriber: Subscriber
  onSaved: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation("subscribers")
  const [attributes, setAttributes] = useState<Record<string, unknown>>(
    subscriber.attributes,
  )
  const [attrsValid, setAttrsValid] = useState(true)
  const [state, setState] = useState(subscriber.state)

  const update = useMutation({
    mutationFn: (v: { name: string }) =>
      api.updateSubscriber(slug, subscriber.id, {
        name: v.name.trim(),
        attributes,
        state,
      }),
    onSuccess: () => {
      toast.success(t("edit.success"))
      onSaved()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: subscriber.name },
    onSubmit: async ({ value }) => {
      if (!attrsValid) return
      await update.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{subscriber.email}</CardTitle>
      </CardHeader>
      <CardContent>
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
              onBlur: compose(rules.required(t("edit.nameRequired"))),
            }}
          >
            {(field) => (
              <FormField
                label={t("edit.name")}
                required
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <FormField label={t("edit.state")}>
            <Select
              value={state}
              onValueChange={(v) => {
                const next = optionOf(STATES, v)
                if (next) setState(next)
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {STATES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <JsonAttributeEditor
            value={attributes}
            onChange={setAttributes}
            onValidityChange={setAttrsValid}
          />
          <div className="flex justify-between">
            <Button type="submit" disabled={update.isPending || !attrsValid}>
              {update.isPending ? t("edit.saving") : t("edit.save")}
            </Button>
            <Button type="button" variant="destructive" onClick={onDelete}>
              <Trash2Icon /> {t("edit.delete")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function MembershipsCard({
  slug,
  subscriber,
  lists,
  onChanged,
}: {
  slug: string
  subscriber: Subscriber
  lists: Array<{ id: string; name: string }>
  onChanged: () => void
}) {
  const { t } = useTranslation("subscribers")
  const [addListId, setAddListId] = useState("")
  const listName = (id: string) =>
    lists.find((l) => l.id === id)?.name ?? id

  const addTo = useMutation({
    mutationFn: (listId: string) =>
      api.addToList(slug, subscriber.id, listId),
    onSuccess: () => {
      toast.success(t("memberships.addSuccess"))
      setAddListId("")
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const removeFrom = useMutation({
    mutationFn: (listId: string) =>
      api.removeFromList(slug, subscriber.id, listId),
    onSuccess: () => {
      toast.success(t("memberships.removeSuccess"))
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const changeStatus = useMutation({
    mutationFn: (v: { listId: string; status: SubscriptionStatus }) =>
      api.changeSubscription(slug, subscriber.id, v.listId, v.status),
    onSuccess: () => {
      toast.success(t("memberships.statusSuccess"))
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const memberListIds = new Set(subscriber.memberships.map((m) => m.listId))
  const available = lists.filter((l) => !memberListIds.has(l.id))

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("memberships.title")}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {subscriber.memberships.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("memberships.empty")}
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {subscriber.memberships.map((m) => (
              <div
                key={m.listId}
                className="flex items-center gap-2 rounded-lg border p-2"
              >
                <span className="flex-1 text-sm font-medium">
                  {listName(m.listId)}
                </span>
                <Select
                  value={m.status}
                  onValueChange={(v) => {
                    const status = optionOf(SUB_STATES, v)
                    if (status) changeStatus.mutate({ listId: m.listId, status })
                  }}
                >
                  <SelectTrigger className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {SUB_STATES.map((s) => (
                        <SelectItem key={s} value={s}>
                          {s}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("memberships.removeFromList")}
                  onClick={() => removeFrom.mutate(m.listId)}
                >
                  <XIcon />
                </Button>
              </div>
            ))}
          </div>
        )}

        {available.length > 0 && (
          <div className="flex items-center gap-2">
            <Select value={addListId} onValueChange={setAddListId}>
              <SelectTrigger className="w-56">
                <SelectValue placeholder={t("memberships.addPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {available.map((l) => (
                    <SelectItem key={l.id} value={l.id}>
                      {l.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Button
              type="button"
              disabled={!addListId || addTo.isPending}
              onClick={() => addTo.mutate(addListId)}
            >
              {t("memberships.add")}
            </Button>
          </div>
        )}

        <p className="text-xs text-muted-foreground">
          <Badge variant="secondary">{subscriber.memberships.length}</Badge>{" "}
          {t("memberships.count", { count: subscriber.memberships.length })}
        </p>
      </CardContent>
    </Card>
  )
}

import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon, XIcon } from "lucide-react"
import { toast } from "sonner"
import type { Subscriber } from "@/lib/api-types"
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

const STATES = ["enabled", "disabled"]
const SUB_STATES = ["subscribed", "unsubscribed"]

export function SubscriberDetail() {
  const { slug, id } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const subscriberQuery = useQuery({
    queryKey: queryKeys.subscriber(slug, id),
    queryFn: async () => (await api.getSubscriber(slug, id)).data.subscriber,
  })

  const listsQuery = useQuery({
    queryKey: queryKeys.lists(slug),
    queryFn: async () =>
      (await api.listLists(slug, { limit: 100, offset: 0 })).data.lists,
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
      toast.success("Subscriber deleted.")
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
          ← Subscribers
        </Link>
      </div>

      <AsyncState query={subscriberQuery}>
        {(subscriber) => (
          <div className="flex flex-col gap-6">
            <EditSubscriberCard
              key={subscriber.ID}
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
        title="Delete this subscriber?"
        description="The subscriber and all their list memberships will be removed."
        confirmLabel="Delete subscriber"
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
  const [attributes, setAttributes] = useState<Record<string, unknown>>(
    subscriber.Attributes,
  )
  const [attrsValid, setAttrsValid] = useState(true)
  const [state, setState] = useState(subscriber.State)

  const update = useMutation({
    mutationFn: (v: { name: string }) =>
      api.updateSubscriber(slug, subscriber.ID, {
        name: v.name.trim(),
        attributes,
        state,
      }),
    onSuccess: () => {
      toast.success("Subscriber updated.")
      onSaved()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: subscriber.Name },
    onSubmit: async ({ value }) => {
      if (!attrsValid) return
      await update.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{subscriber.Email}</CardTitle>
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
            validators={{ onBlur: compose(rules.required("Enter a name.")) }}
          >
            {(field) => (
              <FormField
                label="Name"
                required
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <FormField label="State">
            <Select value={state} onValueChange={setState}>
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
              {update.isPending ? "Saving…" : "Save changes"}
            </Button>
            <Button type="button" variant="destructive" onClick={onDelete}>
              <Trash2Icon /> Delete
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
  lists: Array<{ ID: string; Name: string }>
  onChanged: () => void
}) {
  const [addListId, setAddListId] = useState("")
  const listName = (id: string) =>
    lists.find((l) => l.ID === id)?.Name ?? id

  const addTo = useMutation({
    mutationFn: (listId: string) =>
      api.addToList(slug, subscriber.ID, listId),
    onSuccess: () => {
      toast.success("Added to list.")
      setAddListId("")
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const removeFrom = useMutation({
    mutationFn: (listId: string) =>
      api.removeFromList(slug, subscriber.ID, listId),
    onSuccess: () => {
      toast.success("Removed from list.")
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const changeStatus = useMutation({
    mutationFn: (v: { listId: string; status: string }) =>
      api.changeSubscription(slug, subscriber.ID, v.listId, v.status),
    onSuccess: () => {
      toast.success("Subscription updated.")
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const memberListIds = new Set(subscriber.Memberships.map((m) => m.ListID))
  const available = lists.filter((l) => !memberListIds.has(l.ID))

  return (
    <Card>
      <CardHeader>
        <CardTitle>List memberships</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {subscriber.Memberships.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Not a member of any list yet.
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {subscriber.Memberships.map((m) => (
              <div
                key={m.ListID}
                className="flex items-center gap-2 rounded-lg border p-2"
              >
                <span className="flex-1 text-sm font-medium">
                  {listName(m.ListID)}
                </span>
                <Select
                  value={m.Status}
                  onValueChange={(status) =>
                    changeStatus.mutate({ listId: m.ListID, status })
                  }
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
                  aria-label="Remove from list"
                  onClick={() => removeFrom.mutate(m.ListID)}
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
                <SelectValue placeholder="Add to a list…" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {available.map((l) => (
                    <SelectItem key={l.ID} value={l.ID}>
                      {l.Name}
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
              Add
            </Button>
          </div>
        )}

        <p className="text-xs text-muted-foreground">
          <Badge variant="secondary">{subscriber.Memberships.length}</Badge> list
          membership{subscriber.Memberships.length === 1 ? "" : "s"}
        </p>
      </CardContent>
    </Card>
  )
}

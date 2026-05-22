// Subscriber-field registry CRUD (Phase 7). Gated by
// `subscriber_fields:manage`. Lists built-in pseudo-rows alongside the
// tenant's custom rows — only the custom rows are editable / reorderable /
// deletable. The endpoint signatures live in `api.subscriberFields.*`.

import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import type {
  CreateFieldInput,
  Field,
  FieldType,
  UpdateFieldInput,
} from "@/lib/api-types"
import { FIELD_TYPES } from "@/lib/api-types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/query"
import { errorMessage } from "@/lib/errors"
import { usePermissions } from "@/hooks/use-permissions"
import { Button } from "@/components/ui/button"
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { AsyncState } from "@/components/common/async-state"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import {
  FormField,
  compose,
  fieldError,
  rules,
} from "@/components/common/form-field"

export const Route = createFileRoute("/t/$slug/settings/fields/")({
  component: SubscriberFieldsView,
})

export function SubscriberFieldsView() {
  const { slug } = Route.useParams()
  const { t } = useTranslation("settings")
  const { can } = usePermissions(slug)
  const canManage = can("subscriber_fields:manage")

  const query = useQuery({
    queryKey: queryKeys.subscriberFields(slug),
    queryFn: async () => (await api.subscriberFields.list(slug)).data.fields,
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold">
            {t("fields.page.title")}
          </h1>
          <p className="text-sm text-muted-foreground">
            {t("fields.page.description")}
          </p>
        </div>
      </header>

      {!canManage ? (
        <p
          className="text-sm text-muted-foreground"
          data-testid="ve-fields-forbidden"
        >
          {t("fields.forbidden")}
        </p>
      ) : (
        <AsyncState query={query}>
          {(fields) => (
            <SubscriberFieldsTable slug={slug} fields={fields} />
          )}
        </AsyncState>
      )}
    </div>
  )
}

function SubscriberFieldsTable({
  slug,
  fields,
}: {
  slug: string
  fields: Array<Field>
}) {
  const queryClient = useQueryClient()
  const { t } = useTranslation("settings")
  const [editing, setEditing] = useState<Field | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<Field | null>(null)

  async function invalidate() {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.subscriberFields(slug),
    })
    await queryClient.invalidateQueries({
      queryKey: queryKeys.mergeTags(slug),
    })
  }

  const remove = useMutation({
    mutationFn: (id: string) => api.subscriberFields.delete(slug, id),
    onSuccess: async () => {
      await invalidate()
      toast.success(t("fields.toast.deleted"))
      setDeleting(null)
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      setDeleting(null)
    },
  })

  const reorder = useMutation({
    mutationFn: (order: Array<string>) =>
      api.subscriberFields.reorder(slug, order),
    onSuccess: invalidate,
    onError: (e) => toast.error(errorMessage(e)),
  })

  const custom = fields.filter((f) => !f.builtIn)

  function move(id: string, direction: -1 | 1) {
    const idx = custom.findIndex((f) => f.id === id)
    const next = idx + direction
    if (idx < 0 || next < 0 || next >= custom.length) return
    const reorderedIds = custom.map((f) => f.id)
    ;[reorderedIds[idx], reorderedIds[next]] = [
      reorderedIds[next],
      reorderedIds[idx],
    ]
    reorder.mutate(reorderedIds)
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle>{t("fields.card.title")}</CardTitle>
          <CardDescription>{t("fields.card.description")}</CardDescription>
        </div>
        <Button
          type="button"
          onClick={() => setCreating(true)}
          data-testid="ve-fields-add"
        >
          {t("fields.addField")}
        </Button>
      </CardHeader>
      <CardContent>
        <table className="w-full text-sm" data-testid="ve-fields-table">
          <thead>
            <tr className="text-left text-muted-foreground">
              <th className="py-2">{t("fields.table.displayName")}</th>
              <th>{t("fields.table.slug")}</th>
              <th>{t("fields.table.type")}</th>
              <th>{t("fields.table.default")}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {fields.map((field) => (
              <tr
                key={field.id}
                data-testid={`ve-fields-row-${field.slug}`}
                className="border-t"
              >
                <td className="py-2">{field.displayName}</td>
                <td>
                  <code>{field.slug}</code>
                </td>
                <td>{field.type}</td>
                <td>
                  {field.defaultValue || t("fields.table.emptyDefault")}
                </td>
                <td className="text-right">
                  {field.builtIn ? (
                    <span className="text-xs text-muted-foreground">
                      {t("fields.row.builtIn")}
                    </span>
                  ) : (
                    <div className="flex justify-end gap-1">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-label={t("fields.row.moveUp", {
                          name: field.displayName,
                        })}
                        onClick={() => move(field.id, -1)}
                        data-testid={`ve-fields-up-${field.slug}`}
                      >
                        ↑
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-label={t("fields.row.moveDown", {
                          name: field.displayName,
                        })}
                        onClick={() => move(field.id, 1)}
                        data-testid={`ve-fields-down-${field.slug}`}
                      >
                        ↓
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setEditing(field)}
                        data-testid={`ve-fields-edit-${field.slug}`}
                      >
                        {t("fields.row.edit")}
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setDeleting(field)}
                        data-testid={`ve-fields-delete-${field.slug}`}
                      >
                        {t("fields.row.delete")}
                      </Button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>

      {creating ? (
        <FieldDialog
          slug={slug}
          mode="create"
          onClose={() => setCreating(false)}
          onSaved={invalidate}
        />
      ) : null}

      {editing ? (
        <FieldDialog
          slug={slug}
          mode="edit"
          field={editing}
          onClose={() => setEditing(null)}
          onSaved={invalidate}
        />
      ) : null}

      <ConfirmDialog
        open={deleting != null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title={t("fields.delete.title", {
          name: deleting?.displayName ?? "",
        })}
        description={t("fields.delete.description")}
        confirmLabel={t("fields.delete.confirmLabel")}
        busy={remove.isPending}
        onConfirm={() => {
          if (deleting) remove.mutate(deleting.id)
        }}
      />
    </Card>
  )
}

type DialogProps =
  | {
      slug: string
      mode: "create"
      onClose: () => void
      onSaved: () => Promise<void>
    }
  | {
      slug: string
      mode: "edit"
      field: Field
      onClose: () => void
      onSaved: () => Promise<void>
    }

function FieldDialog(props: DialogProps) {
  const { slug, mode, onClose, onSaved } = props
  const { t } = useTranslation(["settings", "common"])
  const create = useMutation({
    mutationFn: (body: CreateFieldInput) =>
      api.subscriberFields.create(slug, body),
    onSuccess: async () => {
      await onSaved()
      toast.success(t("fields.toast.created"))
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  const update = useMutation({
    mutationFn: (body: UpdateFieldInput) => {
      if (mode !== "edit") {
        return Promise.reject(new Error("not in edit mode"))
      }
      return api.subscriberFields.update(slug, props.field.id, body)
    },
    onSuccess: async () => {
      await onSaved()
      toast.success(t("fields.toast.saved"))
      onClose()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const initial: CreateFieldInput =
    mode === "edit"
      ? {
          slug: props.field.slug,
          displayName: props.field.displayName,
          type: props.field.type,
          defaultValue: props.field.defaultValue,
        }
      : { slug: "", displayName: "", type: "text", defaultValue: "" }

  const form = useForm({
    defaultValues: initial,
    onSubmit: async ({ value }) => {
      if (mode === "create") {
        await create.mutateAsync(value).catch(() => {})
      } else {
        await update
          .mutateAsync({
            displayName: value.displayName,
            type: value.type,
            defaultValue: value.defaultValue,
          })
          .catch(() => {})
      }
    },
  })

  return (
    <div
      role="dialog"
      aria-modal="true"
      data-testid={`ve-fields-${mode}-dialog`}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.4)",
        zIndex: 70,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          form.handleSubmit()
        }}
        style={{
          background: "white",
          padding: 16,
          borderRadius: 8,
          width: 420,
          display: "flex",
          flexDirection: "column",
          gap: 12,
        }}
      >
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>
          {mode === "create"
            ? t("fields.dialog.createTitle")
            : t("fields.dialog.editTitle")}
        </h2>
        <form.Field
          name="slug"
          validators={{
            onBlur:
              mode === "create"
                ? compose(rules.required(t("fields.dialog.slug.required")))
                : undefined,
          }}
        >
          {(field) => (
            <FormField
              label={t("fields.dialog.slug.label")}
              required={mode === "create"}
              value={field.state.value}
              disabled={mode === "edit"}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
              error={fieldError(field.state.meta.errors)}
            />
          )}
        </form.Field>
        <form.Field
          name="displayName"
          validators={{
            onBlur: compose(
              rules.required(t("fields.dialog.displayName.required")),
            ),
          }}
        >
          {(field) => (
            <FormField
              label={t("fields.dialog.displayName.label")}
              required
              value={field.state.value}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
              error={fieldError(field.state.meta.errors)}
            />
          )}
        </form.Field>
        <form.Field name="type">
          {(field) => (
            <div className="flex flex-col gap-1.5">
              <label>{t("fields.dialog.type.label")}</label>
              <Select
                value={field.state.value}
                onValueChange={(v) => field.handleChange(v as FieldType)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {FIELD_TYPES.map((ft) => (
                    <SelectItem key={ft} value={ft}>
                      {ft}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </form.Field>
        <form.Field name="defaultValue">
          {(field) => (
            <FormField
              label={t("fields.dialog.defaultValue.label")}
              value={field.state.value}
              onChange={(e) => field.handleChange(e.target.value)}
            />
          )}
        </form.Field>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common:actions.cancel")}
          </Button>
          <Button
            type="submit"
            disabled={create.isPending || update.isPending}
          >
            {create.isPending || update.isPending
              ? t("common:actions.saving")
              : t("common:actions.save")}
          </Button>
        </div>
      </form>
    </div>
  )
}

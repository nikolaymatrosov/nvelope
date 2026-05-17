import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { errorMessage, isConflict } from "@/lib/errors"
import { queryClient } from "@/lib/query"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/tenants/new")({ component: NewTenant })

function slugify(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
}

function NewTenant() {
  const navigate = useNavigate()
  const [formError, setFormError] = useState("")
  const [slugTaken, setSlugTaken] = useState(false)
  const slugTouched = useRef(false)

  const create = useMutation({
    mutationFn: (v: { name: string; slug: string }) =>
      api.createTenant(v.name.trim(), v.slug.trim()),
    onError: (e) => {
      if (isConflict(e)) {
        setSlugTaken(true)
        return
      }
      setFormError(errorMessage(e))
    },
  })

  const form = useForm({
    defaultValues: { name: "", slug: "" },
    onSubmit: async ({ value }) => {
      setFormError("")
      setSlugTaken(false)
      try {
        await create.mutateAsync(value)
        await queryClient.invalidateQueries({ queryKey: ["me"] })
        await navigate({ to: "/t/$slug", params: { slug: value.slug.trim() } })
      } catch {
        // surfaced via mutation onError
      }
    },
  })

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Create a workspace</CardTitle>
          <CardDescription>
            A workspace holds your lists, subscribers, and team.
          </CardDescription>
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
            {formError && (
              <Alert variant="destructive">
                <AlertTitle>Could not create the workspace</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}
            <form.Field
              name="name"
              validators={{
                onBlur: compose(rules.required("Enter a workspace name.")),
              }}
              listeners={{
                onChange: ({ value }) => {
                  if (!slugTouched.current) {
                    form.setFieldValue("slug", slugify(value))
                  }
                },
              }}
            >
              {(field) => (
                <FormField
                  label="Workspace name"
                  required
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field
              name="slug"
              validators={{
                onBlur: compose(
                  rules.required("Enter a workspace address."),
                  rules.slug(),
                ),
              }}
            >
              {(field) => (
                <FormField
                  label="Workspace address"
                  required
                  hint="Used in URLs — lowercase letters, numbers, and hyphens."
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => {
                    slugTouched.current = true
                    setSlugTaken(false)
                    field.handleChange(e.target.value)
                  }}
                  error={
                    slugTaken
                      ? "That workspace address is already taken."
                      : fieldError(field.state.meta.errors)
                  }
                />
              )}
            </form.Field>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Creating…" : "Create workspace"}
            </Button>
          </form>
        </CardContent>
        <CardFooter>
          <Link
            className="text-sm text-primary underline-offset-4 hover:underline"
            to="/"
          >
            Back to your workspaces
          </Link>
        </CardFooter>
      </Card>
    </main>
  )
}

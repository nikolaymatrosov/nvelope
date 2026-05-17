import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation, useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { errorMessage } from "@/lib/errors"
import { queryClient, queryKeys } from "@/lib/query"
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
import { Spinner } from "@/components/ui/spinner"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/invite/$token")({ component: AcceptInvite })

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">{children}</Card>
    </main>
  )
}

export function AcceptInvite() {
  const { token } = Route.useParams()
  const navigate = useNavigate()
  const [formError, setFormError] = useState("")

  const lookup = useQuery({
    queryKey: queryKeys.invitationLookup(token),
    queryFn: async () => (await api.getInvitation(token)).data,
    retry: false,
  })

  const accept = useMutation({
    mutationFn: (v: { name: string; password: string }) =>
      api.acceptInvitation(token, v.password, v.name.trim()),
    onError: (e) => setFormError(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { name: "", password: "" },
    onSubmit: async ({ value }) => {
      setFormError("")
      try {
        const res = await accept.mutateAsync(value)
        queryClient.clear()
        await navigate({
          to: "/t/$slug",
          params: { slug: res.data.tenant.slug },
        })
      } catch {
        // surfaced via mutation onError
      }
    },
  })

  if (lookup.isLoading) {
    return (
      <main className="grid min-h-svh place-items-center">
        <Spinner className="size-6 text-muted-foreground" />
      </main>
    )
  }

  const invitation = lookup.data
  const isUsable = !lookup.isError && invitation?.status === "pending"

  if (!isUsable) {
    return (
      <Shell>
        <CardHeader>
          <CardTitle>Invitation not valid</CardTitle>
          <CardDescription>
            This invitation has expired, been revoked, or already been used.
          </CardDescription>
        </CardHeader>
        <CardFooter>
          <Link
            className="text-sm text-primary underline-offset-4 hover:underline"
            to="/login"
          >
            Go to sign in
          </Link>
        </CardFooter>
      </Shell>
    )
  }

  return (
    <Shell>
      <CardHeader>
        <CardTitle>Join {invitation.tenant.name}</CardTitle>
        <CardDescription>
          You were invited as <strong>{invitation.email}</strong>. Create your
          account to join.
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
              <AlertTitle>Could not accept the invitation</AlertTitle>
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          )}
          <form.Field
            name="name"
            validators={{ onBlur: compose(rules.required("Enter your name.")) }}
          >
            {(field) => (
              <FormField
                label="Your name"
                required
                autoComplete="name"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <form.Field
            name="password"
            validators={{
              onBlur: compose(rules.minLength(8, "Use at least 8 characters.")),
            }}
          >
            {(field) => (
              <FormField
                label="Choose a password"
                type="password"
                required
                autoComplete="new-password"
                hint="At least 8 characters."
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                error={fieldError(field.state.meta.errors)}
              />
            )}
          </form.Field>
          <Button type="submit" disabled={accept.isPending}>
            {accept.isPending ? "Joining…" : "Accept invitation"}
          </Button>
        </form>
      </CardContent>
      <CardFooter>
        <p className="text-xs text-muted-foreground">
          Already have an account? Log in first, then open this link again.
        </p>
      </CardFooter>
    </Shell>
  )
}

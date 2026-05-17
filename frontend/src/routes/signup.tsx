import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
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

export const Route = createFileRoute("/signup")({ component: Signup })

export function Signup() {
  const navigate = useNavigate()
  const [formError, setFormError] = useState("")
  const [emailTaken, setEmailTaken] = useState(false)

  const signup = useMutation({
    mutationFn: (v: { name: string; email: string; password: string }) =>
      api.signup(v.email.trim(), v.password, v.name.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] })
      await navigate({ to: "/tenants/new" })
    },
    onError: (e) => {
      if (isConflict(e)) {
        setEmailTaken(true)
        return
      }
      setFormError(errorMessage(e))
    },
  })

  const form = useForm({
    defaultValues: { name: "", email: "", password: "" },
    onSubmit: async ({ value }) => {
      setFormError("")
      setEmailTaken(false)
      await signup.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Create your nvelope account</CardTitle>
          <CardDescription>
            Sign up, then create your first workspace.
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
                <AlertTitle>Sign-up failed</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}
            <form.Field
              name="name"
              validators={{ onBlur: compose(rules.required("Enter your name.")) }}
            >
              {(field) => (
                <FormField
                  label="Name"
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
              name="email"
              validators={{ onBlur: compose(rules.required(), rules.email()) }}
            >
              {(field) => (
                <FormField
                  label="Email"
                  type="email"
                  required
                  autoComplete="email"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => {
                    setEmailTaken(false)
                    field.handleChange(e.target.value)
                  }}
                  error={
                    emailTaken
                      ? "An account with this email already exists."
                      : fieldError(field.state.meta.errors)
                  }
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
                  label="Password"
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
            <Button type="submit" disabled={signup.isPending}>
              {signup.isPending ? "Creating…" : "Sign up"}
            </Button>
          </form>
        </CardContent>
        <CardFooter>
          <p className="text-sm text-muted-foreground">
            Already have an account?{" "}
            <Link
              className="text-primary underline-offset-4 hover:underline"
              to="/login"
            >
              Log in
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  )
}

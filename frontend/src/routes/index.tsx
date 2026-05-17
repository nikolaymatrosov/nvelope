import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useMutation } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { api } from "@/lib/api"
import { isUnauthorized } from "@/lib/errors"
import { queryClient } from "@/lib/query"
import { useSession } from "@/hooks/use-session"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Spinner } from "@/components/ui/spinner"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"

export const Route = createFileRoute("/")({ component: Home })

function Home() {
  const navigate = useNavigate()
  const { account, user, tenants, isLoading, isError, error } = useSession()

  const logout = useMutation({
    mutationFn: () => api.logout(),
    onSuccess: () => {
      queryClient.clear()
      navigate({ to: "/login" })
    },
  })

  if (isLoading) {
    return (
      <main className="grid min-h-svh place-items-center">
        <Spinner className="size-6 text-muted-foreground" />
      </main>
    )
  }

  if (isError && isUnauthorized(error)) {
    return (
      <main className="grid min-h-svh place-items-center p-6">
        <Card className="w-full max-w-sm">
          <CardHeader>
            <CardTitle>nvelope</CardTitle>
            <CardDescription>
              Sign in to manage your workspaces.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex gap-3">
            <Button onClick={() => navigate({ to: "/login" })}>Log in</Button>
            <Button
              variant="outline"
              onClick={() => navigate({ to: "/signup" })}
            >
              Sign up
            </Button>
          </CardContent>
        </Card>
      </main>
    )
  }

  return (
    <main className="mx-auto flex min-h-svh max-w-2xl flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Your workspaces</h1>
          <p className="text-sm text-muted-foreground">
            Signed in as {user?.name ?? account?.user.email}.
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
        >
          Sign out
        </Button>
      </header>

      {tenants.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <PlusIcon />
            </EmptyMedia>
            <EmptyTitle>No workspaces yet</EmptyTitle>
            <EmptyDescription>
              Create your first workspace to get started.
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button onClick={() => navigate({ to: "/tenants/new" })}>
              Create a workspace
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2">
            {tenants.map((t) => (
              <Link key={t.id} to="/t/$slug" params={{ slug: t.slug }}>
                <Card className="h-full transition-colors hover:border-primary">
                  <CardHeader>
                    <CardTitle className="flex items-center justify-between gap-2">
                      {t.name}
                      <Badge variant="secondary">{t.role}</Badge>
                    </CardTitle>
                    <CardDescription>/{t.slug}</CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            ))}
          </div>
          <Button
            variant="outline"
            className="self-start"
            onClick={() => navigate({ to: "/tenants/new" })}
          >
            <PlusIcon />
            Create a workspace
          </Button>
        </>
      )}
    </main>
  )
}

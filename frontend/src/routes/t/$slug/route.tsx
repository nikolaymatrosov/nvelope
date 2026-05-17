import { Link, Outlet, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Loader2Icon } from "lucide-react"
import { api } from "@/lib/api"
import { ApiError } from "@/lib/errors"
import { AppShell } from "@/components/shell/app-shell"
import { Button } from "@/components/ui/button"
import { useWorkspace } from "@/hooks/use-workspace"
import { TotpChallenge } from "@/components/shell/totp-challenge"

export const Route = createFileRoute("/t/$slug")({ component: WorkspaceLayout })

function CenteredCard({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <main className="mx-auto flex min-h-svh max-w-md flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">{title}</h1>
      {children}
    </main>
  )
}

function Spinner() {
  return (
    <main className="grid min-h-svh place-items-center">
      <Loader2Icon className="size-6 animate-spin text-muted-foreground" />
    </main>
  )
}

export function WorkspaceLayout() {
  const { slug } = Route.useParams()

  const session = useQuery({
    queryKey: ["t", slug, "session"],
    queryFn: async () => (await api.openSession(slug)).data,
    retry: false,
    staleTime: Infinity,
  })

  if (session.isLoading) return <Spinner />

  if (session.isError) {
    const err = session.error
    if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
      return (
        <CenteredCard title="Workspace not available">
          <p className="text-sm text-muted-foreground">
            This workspace does not exist, or you are not a member of it.
          </p>
          <Button asChild>
            <Link to="/">Back to your workspaces</Link>
          </Button>
        </CenteredCard>
      )
    }
    return (
      <CenteredCard title="Could not open the workspace">
        <p className="text-sm text-muted-foreground">
          Something went wrong opening this workspace. Please try again.
        </p>
        <Button onClick={() => session.refetch()}>Try again</Button>
      </CenteredCard>
    )
  }

  if (session.data?.state === "totp_pending") {
    return (
      <TotpChallenge
        slug={slug}
        onVerified={() => session.refetch()}
      />
    )
  }

  return <WorkspaceShell slug={slug} />
}

function WorkspaceShell({ slug }: { slug: string }) {
  const { name, isLoading, isError, error } = useWorkspace(slug)

  if (isLoading) return <Spinner />

  if (isError) {
    if (
      error instanceof ApiError &&
      (error.status === 404 || error.status === 403)
    ) {
      return (
        <CenteredCard title="Workspace not available">
          <p className="text-sm text-muted-foreground">
            This workspace does not exist, or you are not a member of it.
          </p>
          <Button asChild>
            <Link to="/">Back to your workspaces</Link>
          </Button>
        </CenteredCard>
      )
    }
    return (
      <CenteredCard title="Could not load the workspace">
        <p className="text-sm text-muted-foreground">
          Something went wrong. Please try again.
        </p>
      </CenteredCard>
    )
  }

  return (
    <AppShell slug={slug} workspaceName={name ?? "Workspace"}>
      <Outlet />
    </AppShell>
  )
}

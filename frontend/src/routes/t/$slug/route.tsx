import { Link, Outlet, createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
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
  const { t } = useTranslation()

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
        <CenteredCard title={t("workspace.notAvailableTitle")}>
          <p className="text-sm text-muted-foreground">
            {t("workspace.notAvailableDescription")}
          </p>
          <Button asChild>
            <Link to="/">{t("workspace.backToWorkspaces")}</Link>
          </Button>
        </CenteredCard>
      )
    }
    return (
      <CenteredCard title={t("workspace.openErrorTitle")}>
        <p className="text-sm text-muted-foreground">
          {t("workspace.openErrorDescription")}
        </p>
        <Button onClick={() => session.refetch()}>
          {t("actions.tryAgain")}
        </Button>
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
  const { t } = useTranslation()

  if (isLoading) return <Spinner />

  if (isError) {
    if (
      error instanceof ApiError &&
      (error.status === 404 || error.status === 403)
    ) {
      return (
        <CenteredCard title={t("workspace.notAvailableTitle")}>
          <p className="text-sm text-muted-foreground">
            {t("workspace.notAvailableDescription")}
          </p>
          <Button asChild>
            <Link to="/">{t("workspace.backToWorkspaces")}</Link>
          </Button>
        </CenteredCard>
      )
    }
    return (
      <CenteredCard title={t("workspace.loadErrorTitle")}>
        <p className="text-sm text-muted-foreground">
          {t("workspace.loadErrorDescription")}
        </p>
      </CenteredCard>
    )
  }

  return (
    <AppShell slug={slug} workspaceName={name ?? t("workspace.fallbackName")}>
      <Outlet />
    </AppShell>
  )
}

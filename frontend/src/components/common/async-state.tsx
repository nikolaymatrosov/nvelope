// One wrapper for the loading / empty / error / populated state matrix every
// async view must render (FR-034) — no blank screens.

import { AlertCircleIcon, InboxIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { ReactNode } from "react"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { errorMessage } from "@/lib/errors"

type QueryLike<T> = {
  isLoading: boolean
  isError: boolean
  error: unknown
  data: T | undefined
  refetch?: () => void
}

type AsyncStateProps<T> = {
  query: QueryLike<T>
  children: (data: T) => ReactNode
  isEmpty?: (data: T) => boolean
  loading?: ReactNode
  emptyTitle?: string
  emptyMessage?: string
  emptyAction?: ReactNode
}

function DefaultSkeleton() {
  return (
    <div className="flex flex-col gap-3" data-testid="async-loading">
      <Skeleton className="h-9 w-full" />
      <Skeleton className="h-9 w-full" />
      <Skeleton className="h-9 w-2/3" />
    </div>
  )
}

export function AsyncState<T>({
  query,
  children,
  isEmpty,
  loading,
  emptyTitle,
  emptyMessage,
  emptyAction,
}: AsyncStateProps<T>) {
  const { t } = useTranslation()
  if (query.isLoading) {
    return <>{loading ?? <DefaultSkeleton />}</>
  }

  if (query.isError) {
    return (
      <Empty data-testid="async-error" className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <AlertCircleIcon className="text-destructive" />
          </EmptyMedia>
          <EmptyTitle>{t("asyncState.errorTitle")}</EmptyTitle>
          <EmptyDescription>{errorMessage(query.error)}</EmptyDescription>
        </EmptyHeader>
        {query.refetch && (
          <EmptyContent>
            <Button
              variant="outline"
              size="sm"
              onClick={() => query.refetch?.()}
            >
              {t("actions.tryAgain")}
            </Button>
          </EmptyContent>
        )}
      </Empty>
    )
  }

  if (query.data === undefined) {
    return <>{loading ?? <DefaultSkeleton />}</>
  }

  if (isEmpty && isEmpty(query.data)) {
    return (
      <Empty data-testid="async-empty" className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <InboxIcon />
          </EmptyMedia>
          <EmptyTitle>{emptyTitle ?? t("asyncState.emptyTitle")}</EmptyTitle>
          <EmptyDescription>
            {emptyMessage ?? t("asyncState.emptyMessage")}
          </EmptyDescription>
        </EmptyHeader>
        {emptyAction && <EmptyContent>{emptyAction}</EmptyContent>}
      </Empty>
    )
  }

  return <div data-testid="async-populated">{children(query.data)}</div>
}

// A labelled count with an optional rate line — the building block of the
// campaign analytics view and the workspace dashboard (Phase 4 US1, US3).

import type { ReactNode } from "react"
import { Card, CardContent } from "@/components/ui/card"

type MetricTileProps = {
  label: string
  value: number
  rate?: ReactNode
}

export function MetricTile({ label, value, rate }: MetricTileProps) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {label}
        </p>
        <p className="text-2xl font-semibold tabular-nums">
          {value.toLocaleString()}
        </p>
        {rate !== undefined && (
          <p className="text-sm text-muted-foreground tabular-nums">{rate}</p>
        )}
      </CardContent>
    </Card>
  )
}

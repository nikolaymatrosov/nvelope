// Renders metered sends consumed against a plan's included allowance as a
// proportional bar with a near-limit / exhausted visual cue (Phase 5 US3).

import { Progress } from "@/components/ui/progress"

type UsageMeterProps = {
  used: number
  included: number
}

export function UsageMeter({ used, included }: UsageMeterProps) {
  const ratio = included > 0 ? used / included : used > 0 ? 1 : 0
  const percent = Math.min(100, Math.round(ratio * 100))
  const exhausted = included > 0 && used >= included
  const nearLimit = !exhausted && ratio >= 0.8

  const tone = exhausted
    ? "text-destructive"
    : nearLimit
      ? "text-amber-600"
      : "text-muted-foreground"

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between">
        <span className="text-2xl font-semibold tabular-nums">
          {used.toLocaleString()}
          <span className="text-base font-normal text-muted-foreground">
            {" "}
            / {included.toLocaleString()} sends
          </span>
        </span>
        <span className={`text-sm font-medium tabular-nums ${tone}`}>
          {percent}%
        </span>
      </div>
      <Progress value={percent} />
      <p className={`text-sm ${tone}`}>
        {exhausted
          ? "Plan allowance reached for this period."
          : nearLimit
            ? "Approaching the plan allowance for this period."
            : `${Math.max(0, included - used).toLocaleString()} sends remaining this period.`}
      </p>
    </div>
  )
}

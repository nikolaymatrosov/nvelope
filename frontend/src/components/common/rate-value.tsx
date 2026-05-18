// Renders a backend rate — a fraction in [0, 1] — as a percentage. The backend
// already yields 0.0 for a zero denominator, so a zero rate renders as "0%"
// with no special-casing (Phase 4 FR-005).

type RateValueProps = {
  value: number
  fractionDigits?: number
}

export function RateValue({ value, fractionDigits = 1 }: RateValueProps) {
  const safe = Number.isFinite(value) ? value : 0
  const pct = safe * 100
  // Drop trailing ".0" so a clean rate reads "12%" rather than "12.0%".
  const text = Number.isInteger(pct)
    ? `${pct}%`
    : `${pct.toFixed(fractionDigits)}%`
  return <span>{text}</span>
}

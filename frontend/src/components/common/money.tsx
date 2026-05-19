// Formats a minor-unit integer amount (kopecks, cents) plus an ISO currency
// code into a localized monetary string — the single place billing amounts are
// rendered so rounding and currency display stay consistent (Phase 5 UI).

export function formatMoney(amountMinor: number, currency: string): string {
  const code = currency || "RUB"
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: code,
    }).format(amountMinor / 100)
  } catch {
    return `${(amountMinor / 100).toFixed(2)} ${code}`
  }
}

export function Money({
  amountMinor,
  currency,
}: {
  amountMinor: number
  currency: string
}) {
  return <span className="tabular-nums">{formatMoney(amountMinor, currency)}</span>
}

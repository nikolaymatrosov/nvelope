package domain

// Money is an exact monetary amount: an integer count of minor units (kopecks
// for RUB) plus an ISO 4217 currency code. Storing amounts as integers means no
// floating-point rounding error accumulates across line items, overage
// arithmetic, or retries; arithmetic refuses to mix currencies.
type Money struct {
	minor    int64
	currency string
}

// NewMoney builds a Money of minor units in the given currency.
func NewMoney(minor int64, currency string) Money {
	return Money{minor: minor, currency: currency}
}

// ZeroMoney is the zero amount in the given currency — the identity for Add and
// the starting point for summing line items.
func ZeroMoney(currency string) Money {
	return Money{minor: 0, currency: currency}
}

// Minor returns the amount as a count of minor units.
func (m Money) Minor() int64 { return m.minor }

// Currency returns the ISO 4217 currency code.
func (m Money) Currency() string { return m.currency }

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool { return m.minor == 0 }

// Add returns the sum of two amounts, rejecting a currency mismatch with
// ErrCurrencyMismatch so a cross-currency total is unrepresentable.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{minor: m.minor + other.minor, currency: m.currency}, nil
}

// Mul scales the amount by an integer quantity, keeping the currency.
func (m Money) Mul(qty int64) Money {
	return Money{minor: m.minor * qty, currency: m.currency}
}

// Equal reports whether two amounts are equal in both value and currency.
func (m Money) Equal(other Money) bool {
	return m.minor == other.minor && m.currency == other.currency
}

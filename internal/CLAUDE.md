## Database queries

The pgx adapters under `internal/*/adapters/` follow a placeholder convention:

- **≥ 4 placeholders** → use `pgx.NamedArgs` with `@snake_case` placeholders that
  match the column names. The visual alignment between column list, `VALUES (...)`,
  and the args map makes ordering errors obvious and survives column reordering.
- **< 4 placeholders** (typical `WHERE id = $1`, `LIMIT $1 OFFSET $2`) → keep
  positional `$N`. A `NamedArgs` map literal is noise at that size.

`pgx.NamedArgs` is a thin client-side rewrite to positional params (no server
cost), but it loses compile-time "did I pass N args for N placeholders" checking
— typos in `@foo` become runtime errors. The threshold reflects that trade-off.

// Package adapters implements the tenant domain's repository interfaces against
// pgx, and owns the Row-Level-Security-bound transaction helper; the *_ydb.go
// files are the YDB implementations of the repositories for the serverless
// runtime.
package adapters

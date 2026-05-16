package db

import "embed"

// MigrationsFS holds the versioned SQL migrations, embedded so the migrate
// CLI carries them in its binary.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

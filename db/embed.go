// Package db holds the SQL that defines the database: schema migrations and
// the seed data loaded into a fresh environment.
//
// The files are embedded so that `make migrate` and `make seed` work from any
// working directory and the deployed binaries carry their own SQL.
package db

import "embed"

// Migrations holds the versioned schema changes applied by cmd/migrate.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// Seeds holds the rows loaded by cmd/seed. Files under base/ are loaded in
// every environment; files under dev/ only outside production.
//
//go:embed seeds/base/*.sql seeds/dev/*.sql
var Seeds embed.FS

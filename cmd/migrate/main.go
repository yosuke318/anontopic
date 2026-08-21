// Command migrate applies the schema migrations embedded from db/migrations.
//
// Usage:
//
//	migrate          スキーマを最新にする
//	migrate up       同上
//	migrate down     直前のマイグレーションを 1 つ戻す
//	migrate version  適用済みバージョンを表示する
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/yosuke318/anontopic/db"
)

const defaultDatabaseURL = "postgres://anontopic:anontopic@localhost:5432/anontopic?sslmode=disable"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := run(command); err != nil {
		slog.Error("migration failed", slog.String("command", command), slog.Any("error", err))
		os.Exit(1)
	}
}

func run(command string) error {
	source, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		return err
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Warn("closing migration source", slog.Any("error", sourceErr))
		}
		if dbErr != nil {
			slog.Warn("closing migration database", slog.Any("error", dbErr))
		}
	}()

	switch command {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	case "version":
		return printVersion(m)
	default:
		return fmt.Errorf("unknown command %q: use up, down or version", command)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("schema already up to date")
		return nil
	}
	if err != nil {
		return err
	}

	return printVersion(m)
}

func printVersion(m *migrate.Migrate) error {
	version, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		slog.Info("no migration applied yet")
		return nil
	}
	if err != nil {
		return err
	}

	slog.Info("schema version", slog.Uint64("version", uint64(version)), slog.Bool("dirty", dirty))
	return nil
}

// Command seed loads the rows embedded from db/seeds into the database.
//
// Files under seeds/base are loaded in every environment. Files under
// seeds/dev are loaded unless APP_ENV is "production", so that a local
// environment has conversations to look at right after `make up`.
//
// Every file is written to be safe to run repeatedly.
package main

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/yosuke318/anontopic/db"
)

const defaultDatabaseURL = "postgres://anontopic:anontopic@localhost:5432/anontopic?sslmode=disable"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(context.Background()); err != nil {
		slog.Error("seeding failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	dirs := []string{"seeds/base"}
	if os.Getenv("APP_ENV") != "production" {
		dirs = append(dirs, "seeds/dev")
	}

	for _, dir := range dirs {
		files, err := sqlFiles(dir)
		if err != nil {
			return err
		}

		for _, file := range files {
			statements, err := db.Seeds.ReadFile(file)
			if err != nil {
				return err
			}

			// Exec without arguments uses the simple query protocol, so a file
			// may hold several statements.
			if _, err := conn.Exec(ctx, string(statements)); err != nil {
				return err
			}

			slog.Info("seed applied", slog.String("file", file))
		}
	}

	return nil
}

func sqlFiles(dir string) ([]string, error) {
	entries, err := fs.ReadDir(db.Seeds, dir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, dir+"/"+entry.Name())
		}
	}
	sort.Strings(files)

	return files, nil
}

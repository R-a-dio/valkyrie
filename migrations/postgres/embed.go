package postgres

import (
	"context"
	"embed"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage/postgres"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var FS embed.FS

func New(ctx context.Context, cfg config.Config) (*migrate.Migrate, error) {
	sd, err := Source(ctx, cfg)
	if err != nil {
		return nil, err
	}
	db, err := postgres.ConnectDB(ctx, cfg, true)
	if err != nil {
		return nil, err
	}
	dd, err := pgx.WithInstance(db.DB, &pgx.Config{})
	if err != nil {
		return nil, err
	}

	return migrate.NewWithInstance(
		"embed", sd,
		"postgres", dd,
	)
}

func Source(ctx context.Context, cfg config.Config) (source.Driver, error) {
	return iofs.New(FS, ".")
}

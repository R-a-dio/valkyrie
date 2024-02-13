package migrations

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/migrations/mysql"
	"github.com/golang-migrate/migrate/v4"
)

func New(ctx context.Context, cfg config.Config) (*migrate.Migrate, error) {
	const op errors.Op = "migrations.NewMigration"

	storageProvider := cfg.Conf().Providers.Storage
	switch storageProvider {
	case "mariadb", "mysql":
		return mysql.New(ctx, cfg)
	default:
		return nil, errors.E(op, errors.NoMigrations)
	}
}

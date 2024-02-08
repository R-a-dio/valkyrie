package migrations

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/migrations/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
)

func New(ctx context.Context, cfg config.Config) (*migrate.Migrate, error) {
	const op errors.Op = "migrations.NewMigration"

	var err error
	var files source.Driver
	var driver database.Driver

	driverName := cfg.Conf().Providers.Storage
	switch driverName {
	case "mysql":
		files, driver, err = mysql.New(ctx, cfg)
	default:
		return nil, errors.E(op, errors.NoMigrations)
	}

	if err != nil {
		return nil, err
	}

	return migrate.NewWithInstance(
		"embed", files,
		driverName, driver,
	)
}

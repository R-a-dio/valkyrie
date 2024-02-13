package migrations

import (
	"context"
	"os"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/migrations/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source"
)

// New returns a new migration instance for the configured storage provider if
// available, returns NoMigrations if none exist
func New(ctx context.Context, cfg config.Config) (*migrate.Migrate, error) {
	const op errors.Op = "migrations.NewMigration"

	storageProvider := cfg.Conf().Providers.Storage
	_, newFn, err := selectFunctions(storageProvider)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return newFn(ctx, cfg)
}

type srcFn func(context.Context, config.Config) (source.Driver, error)
type newFn func(context.Context, config.Config) (*migrate.Migrate, error)

func selectFunctions(provider string) (srcFn, newFn, error) {
	switch provider {
	case "mariadb", "mysql":
		return mysql.Source, mysql.New, nil
	default:
		return nil, nil, errors.E(errors.NoMigrations)
	}
}

func CheckVersion(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "migrations.CheckVersion"

	storageProvider := cfg.Conf().Providers.Storage
	sourceFn, newFn, err := selectFunctions(storageProvider)
	if err != nil {
		return errors.E(op, err)
	}

	s, err := sourceFn(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	migr, err := newFn(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	version, err := s.First()
	if err != nil {
		return errors.E(op, err)
	}

	// find the last version available
	latest := version
	for {
		version, err = s.Next(version)
		if err != nil {
			if errors.IsE(err, os.ErrNotExist) {
				break
			} else {
				return errors.E(op, err)
			}
		}
		latest = version
	}

	// ask for the current version from the database
	current, dirty, err := migr.Version()
	if err != nil {
		return errors.E(op, err)
	}
	// check if we're not in a dirty state
	if dirty {
		return errors.E(op, errors.DirtyMigration)
	}
	// then check we're on the latest
	if current != latest {
		return errors.E(op, errors.MigrationNotApplied)
	}

	return nil
}

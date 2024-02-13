package mysql

import (
	"context"
	"embed"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
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
	db, err := mariadb.ConnectDB(ctx, cfg, true)
	if err != nil {
		return nil, err
	}
	dd, err := mysql.WithInstance(db.DB, &mysql.Config{})
	if err != nil {
		return nil, err
	}

	return migrate.NewWithInstance(
		"embed", sd,
		"mysql", dd,
	)
}

func Source(ctx context.Context, cfg config.Config) (source.Driver, error) {
	return iofs.New(FS, ".")
}

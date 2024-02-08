package mysql

import (
	"context"
	"embed"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var FS embed.FS

func New(ctx context.Context, cfg config.Config) (source.Driver, database.Driver, error) {
	sd, err := iofs.New(FS, ".")
	if err != nil {
		return nil, nil, err
	}
	db, err := mariadb.ConnectDB(ctx, cfg, true)
	if err != nil {
		return nil, nil, err
	}
	dd, err := mysql.WithInstance(db.DB, &mysql.Config{})
	if err != nil {
		return nil, nil, err
	}
	return sd, dd, nil
}

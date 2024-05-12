package mariadb_test

import (
	"context"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	mysqlmigrations "github.com/R-a-dio/valkyrie/migrations/mysql"
	"github.com/R-a-dio/valkyrie/storage"
	storagetest "github.com/R-a-dio/valkyrie/storage/test"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

type MariaDBSetup struct {
	container *mariadb.MariaDBContainer
	db        *sqlx.DB
}

func (setup *MariaDBSetup) Setup(ctx context.Context) error {
	testcontainers.Logger = testcontainers.TestLogger(storagetest.CtxT(ctx))

	// setup a container to test in
	container, err := mariadb.RunContainer(ctx,
		testcontainers.WithImage("mariadb:latest"),
		//mariadb.WithDatabase("test"),
		mariadb.WithUsername("root"),
		mariadb.WithPassword(""),
	)
	if err != nil {
		return err
	}
	setup.container = container

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		return err
	}
	dsn = fixdsn(dsn)

	setup.db, err = sqlx.ConnectContext(ctx, "mysql", dsn)
	if err != nil {
		container.Terminate(ctx)
	}
	return err
}

func (setup *MariaDBSetup) RunMigrations(ctx context.Context, cfg config.Config) error {
	migr, err := mysqlmigrations.New(ctx, cfg)
	if err != nil {
		return err
	}

	err = migr.Up()
	if err != nil {
		return err
	}
	return nil
}

func (setup *MariaDBSetup) TearDown(ctx context.Context) error {
	err := setup.container.Terminate(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (setup *MariaDBSetup) CreateStorage(ctx context.Context, name string) (radio.StorageService, error) {
	// test names have a / prefixed sometimes
	name = strings.ReplaceAll(name, "/", "")
	// create the database
	setup.db.MustExecContext(ctx, "CREATE DATABASE "+name+";")
	// update our config to connect to the container
	cfg := config.TestConfig()

	dsn, err := setup.container.ConnectionString(ctx)
	if err != nil {
		return nil, err
	}
	dsn = fixdsn(dsn)

	mycfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}

	mycfg.DBName = name
	bare := cfg.Conf()
	bare.Database.DSN = mycfg.FormatDSN()
	cfg.StoreConf(bare)

	// run migrations
	err = setup.RunMigrations(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// then open a storage instance
	s, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func TestMariaDBStorage(t *testing.T) {
	if !testing.Short() {
		storagetest.RunTests(t, new(MariaDBSetup))
	}
}

func fixdsn(dsn string) string {
	// see https://github.com/testcontainers/testcontainers-go/issues/551
	// using localhost as the address will run a racer on ipv4
	// and ipv6 and connect to whoever wins, but docker doesn't
	// actually (sometimes) bind both to the same port so the
	// ipv6 version isn't valid to use. so we fix it by forcing ipv4
	return strings.Replace(dsn, "localhost", "127.0.0.1", 1)
}

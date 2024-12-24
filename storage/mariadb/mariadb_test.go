package mariadb_test

import (
	"context"
	"log"
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
	dbname    string
	db        *sqlx.DB

	truncateQuery string
	storage       radio.StorageService
}

func (setup *MariaDBSetup) Setup(ctx context.Context) error {
	t := storagetest.CtxT(ctx)
	testcontainers.Logger = testcontainers.TestLogger(t)

	setup.dbname = "go-test"
	// setup a container to test in
	container, err := mariadb.Run(ctx, "mariadb:latest",
		mariadb.WithDatabase(setup.dbname),
		mariadb.WithUsername("root"),
		mariadb.WithPassword(""),
	)
	if err != nil {
		return err
	}
	setup.container = container

	// setup the DSN
	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		return err
	}
	dsn = fixdsn(dsn)
	mycfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return err
	}
	mycfg.MultiStatements = true
	dsn = mycfg.FormatDSN()

	// setup the config
	cfg := config.TestConfig()
	bare := cfg.Conf()
	bare.Database.DSN = dsn
	cfg.StoreConf(bare)

	// connect
	setup.db, err = sqlx.ConnectContext(ctx, "mysql", dsn)
	if err != nil {
		return err
	}

	err = setup.RunMigrations(ctx, cfg)
	if err != nil {
		return err
	}

	var names []string
	err = sqlx.Select(setup.db, &names, "SELECT table_name FROM information_schema.tables WHERE table_schema = ?", setup.dbname)
	if err != nil {
		return err
	}

	setup.truncateQuery = "SET FOREIGN_KEY_CHECKS=0;"
	for _, name := range names {
		if name == "schema_migrations" {
			// this one is where the migrations package stores its stuff, don't truncate
			// that one
			continue
		}
		if name == "permission_kinds" {
			// permission_kinds are constant values too, don't delete these between tests
			continue
		}

		setup.truncateQuery += "TRUNCATE TABLE " + name + "; "
	}
	setup.truncateQuery += "SET FOREIGN_KEY_CHECKS=1;"

	setup.storage, err = storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (setup *MariaDBSetup) RunMigrations(ctx context.Context, cfg config.Config) error {
	migr, err := mysqlmigrations.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer migr.Close()

	err = migr.Up()
	if err != nil {
		return err
	}
	return nil
}

func (setup *MariaDBSetup) TearDown(ctx context.Context) error {
	if setup.storage != nil {
		setup.storage.Close()
	}
	if setup.db != nil {
		setup.db.Close()
	}
	if setup.container != nil {
		err := setup.container.Terminate(ctx)
		if err != nil {
			log.Println("error terminating testcontainer:", err)
		}
	}

	return nil
}

func (setup *MariaDBSetup) CreateStorage(ctx context.Context) radio.StorageService {
	// truncate all tables in the database
	sqlx.MustExecContext(ctx, setup.db, setup.truncateQuery)
	return setup.storage
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

package postgres_test

import (
	"context"
	"log"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/migrations"
	"github.com/R-a-dio/valkyrie/storage"
	storagetest "github.com/R-a-dio/valkyrie/storage/test"

	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go"
	tlog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type PostgresSetup struct {
	container *postgres.PostgresContainer
	dbname    string
	db        *sqlx.DB

	truncateQuery string
	storage       radio.StorageService
}

func (setup *PostgresSetup) Setup(ctx context.Context) error {
	t := storagetest.CtxT(ctx)
	tlog.SetDefault(tlog.TestLogger(t))

	setup.dbname = "go-test"
	// setup a container to test in
	container, err := postgres.Run(ctx, "postgres:latest",
		postgres.WithDatabase(setup.dbname),
		postgres.WithUsername("root"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
		postgres.WithSQLDriver("pgx"),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		return err
	}
	setup.container = container

	// setup the DSN
	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		t.Log("failed to retrieve connection DSN")
		return err
	}
	dsn = fixdsnfordocker(dsn)
	t.Log("DSN", dsn)

	// setup the config
	cfg := config.TestConfig()
	bare := cfg.Conf()
	bare.Database.DSN = dsn
	bare.Database.DriverName = "pgx"
	bare.Providers.Storage = "postgres"
	cfg.StoreConf(bare)

	// connect
	setup.db, err = sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		t.Log("failed to connect to postgres")
		return err
	}

	err = setup.RunMigrations(ctx, cfg)
	if err != nil {
		t.Log("failed to run migrations")
		return err
	}
	// this db isn't needed anymore
	setup.db.Close()

	// now make a snapshot we can restore after each test
	err = container.Snapshot(ctx)
	if err != nil {
		t.Log("failed to create snapshot")
		return err
	}

	setup.storage, err = storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (setup *PostgresSetup) RunMigrations(ctx context.Context, cfg config.Config) error {
	migr, err := migrations.New(ctx, cfg)
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

func (setup *PostgresSetup) TearDown(ctx context.Context) error {
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

func (setup *PostgresSetup) CreateStorage(ctx context.Context) radio.StorageService {
	// truncate all tables in the database
	err := setup.container.Restore(ctx)
	if err != nil {
		panic("failed to restore postgres snapshot")
	}

	return setup.storage
}

func TestPostgresStorage(t *testing.T) {
	if !testing.Short() {
		storagetest.RunTests(t, new(PostgresSetup))
	}
}

func fixdsnfordocker(dsn string) string {
	// see https://github.com/testcontainers/testcontainers-go/issues/551
	// using localhost as the address will run a racer on ipv4
	// and ipv6 and connect to whoever wins, but docker doesn't
	// actually (sometimes) bind both to the same port so the
	// ipv6 version isn't valid to use. so we fix it by forcing ipv4
	return strings.Replace(dsn, "localhost", "127.0.0.1", 1)
}

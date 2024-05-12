package migrations

import (
	"context"
	"testing"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

func TestCheckVersion(t *testing.T) {
	ctx := context.Background()

	testcontainers.Logger = testcontainers.TestLogger(t)

	// setup a container to test in
	container, err := mariadb.RunContainer(ctx,
		testcontainers.WithImage("mariadb:latest"),
		mariadb.WithDatabase("test"),
		mariadb.WithUsername("root"),
		mariadb.WithPassword(""),
	)
	require.NoError(t, err, "failed setting up container")

	cfg := config.TestConfig()

	dsn, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	c := cfg.Conf()
	c.Database.DSN = dsn
	cfg.StoreConf(c)

	// first CheckVersion, should fail because no migrations have been ran yet
	err = CheckVersion(ctx, cfg)
	require.Error(t, err)
	require.ErrorIs(t, err, migrate.ErrNilVersion)

	migr, err := New(ctx, cfg)
	require.NoError(t, err)

	// now run all the migrations
	err = migr.Up()
	require.NoError(t, err)

	// then CheckVersion again that should succeed
	err = CheckVersion(ctx, cfg)
	require.NoError(t, err)
}

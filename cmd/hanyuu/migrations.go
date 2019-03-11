package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	migrations "github.com/R-a-dio/valkyrie/migrations/embed"
	"github.com/R-a-dio/valkyrie/storage/mariadb"

	"github.com/golang-migrate/migrate/v4"
	mysqldriver "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/google/subcommands"
)

type migrateCmd struct {
	// command line flags
	flags   *flag.FlagSet
	verbose bool

	// migration drivers
	source  source.Driver
	migrate *migrate.Migrate
}

func (m migrateCmd) Name() string {
	return "migrate"
}

func (m migrateCmd) Synopsis() string {
	return "migrate allows you to handle database migrations"
}

func (m migrateCmd) Usage() string {
	return "migrate"
}

func (m *migrateCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&m.verbose, "verbose", false, "verbose output")
}

func (m *migrateCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	m.flags = f
	defer func() {
		if m.migrate != nil {
			m.migrate.Close()
		}
	}()

	cmder := subcommands.NewCommander(f, path.Base(os.Args[0])+" migrate")
	cmder.Register(cmder.HelpCommand(), "")
	cmder.Register(cmder.CommandsCommand(), "")
	cmder.Register(cmd{
		name:     "version",
		synopsis: "return the current schema version",
		usage: `version:
		return the current schema version
		`,
		execute: withConfig(m.version),
	}, "")
	cmder.Register(cmd{
		name:     "up",
		synopsis: "migrate the schema version up",
		usage: `up:
		migrate the schema version up
		`,
		execute: withConfig(m.up),
	}, "")
	cmder.Register(cmd{
		name:     "force",
		synopsis: "set the current version of the schema, this does not run any migrations",
		usage: `force <version>:
		set the current version of the schema, this does not run any migrations but only
		records what version the schema currently is
		`,
		execute: withConfig(m.forceVersion),
	}, "")
	cmder.Register(cmd{
		name:     "ls",
		synopsis: "shows a list of all migrations",
		usage: `ls:
		shows a list of all migrations
		`,
		execute: m.ls,
	}, "")

	return cmder.Execute(ctx, args...)
}

func (m migrateCmd) up(ctx context.Context, cfg config.Config) error {
	err := m.setup(ctx, cfg)
	if err != nil {
		return err
	}

	return m.migrate.Up()
}

func (m migrateCmd) ls(context.Context, config.Loader) error {
	migr, err := migrations.ListMigrations()
	if err != nil {
		return err
	}

	for i := range migr {
		fmt.Println(migr[i].Pretty())
	}
	return nil
}

func (m migrateCmd) forceVersion(ctx context.Context, cfg config.Config) error {
	err := m.setup(ctx, cfg)
	if err != nil {
		return err
	}

	args := m.flags.Args()
	if len(args) < 2 {
		return errors.New("missing version argument")
	}

	version, err := strconv.Atoi(args[1])
	if err != nil {
		return errors.Errorf("malformed version: %s", err)
	}

	return m.migrate.Force(version)
}

func (m migrateCmd) version(ctx context.Context, cfg config.Config) error {
	err := m.setup(ctx, cfg)
	if err != nil {
		return err
	}

	v, d, err := m.migrate.Version()
	if err != nil {
		return err
	}

	state := "done"
	if d {
		state = "dirty"
	}

	fmt.Printf("version: %d, state: %s\n", v, state)
	return nil
}

func (m *migrateCmd) setup(ctx context.Context, cfg config.Config) error {
	s, err := migrations.Source.Open("")
	if err != nil {
		return err
	}
	m.source = s

	db, err := mariadb.ConnectDB(ctx, cfg, true)
	if err != nil {
		return err
	}

	d, err := mysqldriver.WithInstance(db.DB, &mysqldriver.Config{})
	if err != nil {
		return err
	}

	migr, err := migrate.NewWithInstance(
		"becky", m.source,
		"mysql", d,
	)
	if err != nil {
		return err
	}

	if m.verbose {
		migr.Log = migrateLog{log.New(os.Stderr, "", log.LstdFlags)}
	}
	m.migrate = migr
	return nil
}

type migrateLog struct {
	*log.Logger
}

func (ml migrateLog) Verbose() bool {
	return true
}

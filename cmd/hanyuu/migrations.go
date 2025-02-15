package main

import (
	"context"
	"os"
	"strconv"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/migrations"
	"github.com/spf13/cobra"

	"github.com/golang-migrate/migrate/v4"
)

func MigrationCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "migrate",
		GroupID: "helpers",
		Short:   "tool to manage migrations against the database",
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "return the current schema version",
			Args:  cobra.NoArgs,
			RunE:  SimpleCommand(MigrationVersion),
		},
		&cobra.Command{
			Use:   "up",
			Short: "migrate the schema version up",
			Args:  cobra.NoArgs,
			RunE:  SimpleCommand(MigrationUp),
		},
		&cobra.Command{
			Use:   "force <version>",
			Short: "force the schema version, this does not run any migrations",
			RunE:  SimpleCommand(MigrationForceVersion),
			Args:  cobra.ExactArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				source, err := migrations.NewSource(cmd.Context(), cfgFromContext(cmd.Context()))
				if err != nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}

				var things []string
				for {
					version, err := source.First()
					if err != nil {
						if errors.IsE(err, os.ErrNotExist) {
							break
						}
						return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
					}

					things = append(things, strconv.Itoa(int(version)))
				}

				return things, cobra.ShellCompDirectiveNoFileComp
			},
		},
		&cobra.Command{
			Use:   "is-latest",
			Short: "checks if you're on the latest version of the schema",
			Args:  cobra.NoArgs,
			RunE:  Command(MigrationIsLatestVersion),
		},
		&cobra.Command{
			Use:   "ls",
			Short: "lists all available migrations",
			Args:  cobra.NoArgs,
			RunE:  SimpleCommand(MigrationList),
		},
	)

	return root
}

func MigrationUp(cmd *cobra.Command, args []string) error {
	m, err := MigrationSetup(cmd)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
			m.GracefulStop <- true
		case <-done:
		}
	}(cmd.Context())

	return m.Up()
}

func MigrationList(cmd *cobra.Command, args []string) error {
	source, err := migrations.NewSource(cmd.Context(), cfgFromContext(cmd.Context()))
	if err != nil {
		return err
	}

	version, err := source.First()
	if err != nil {
		return err
	}

	for {
		_, identifier, err := source.ReadUp(version)
		if err != nil && !errors.IsE(err, os.ErrNotExist) {
			return err
		}

		if !errors.IsE(err, os.ErrNotExist) {
			cmd.Printf("%.4d   UP %s\n", version, identifier)
		}

		_, identifier, err = source.ReadDown(version)
		if err != nil && !errors.IsE(err, os.ErrNotExist) {
			return err
		}

		if !errors.IsE(err, os.ErrNotExist) {
			cmd.Printf("%.4d DOWN %s\n", version, identifier)
		}

		version, err = source.Next(version)
		if err != nil {
			if errors.IsE(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
	}
}

func MigrationForceVersion(cmd *cobra.Command, args []string) error {
	m, err := MigrationSetup(cmd)
	if err != nil {
		return err
	}

	version, err := strconv.Atoi(args[1])
	if err != nil {
		return errors.Errorf("malformed version: %s", err)
	}

	return m.Force(version)
}

var MigrationIsLatestVersion = migrations.CheckVersion

func MigrationVersion(cmd *cobra.Command, args []string) error {
	m, err := MigrationSetup(cmd)
	if err != nil {
		return err
	}

	v, d, err := m.Version()
	if err != nil {
		return err
	}

	state := "done"
	if d {
		state = "dirty"
	}

	cmd.Printf("version: %d, state: %s\n", v, state)
	return nil
}

func MigrationSetup(cmd *cobra.Command) (*migrate.Migrate, error) {
	migr, err := migrations.New(cmd.Context(), cfgFromContext(cmd.Context()))
	if err != nil {
		return nil, err
	}

	migr.Log = migrateLog{cmd}
	return migr, nil
}

type migrateLog struct {
	l interface {
		Printf(format string, args ...any)
	}
}

func (ml migrateLog) Printf(format string, args ...any) {
	ml.l.Printf(format, args...)
}

func (ml migrateLog) Verbose() bool {
	return true
}

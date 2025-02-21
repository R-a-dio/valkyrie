package main

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/spf13/cobra"
)

func DatabaseCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "database",
		GroupID: "helpers",
		Short:   "helper commands to add things to the database from the command line",
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "add-tracks <filename>...",
			Short: "add a music file to the database",
			RunE:  SimpleCommand(DatabaseAddTrack),
			Args:  cobra.MinimumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return []string{"flac", "mp3", "opus"}, cobra.ShellCompDirectiveFilterFileExt | cobra.ShellCompDirectiveNoFileComp
			},
		},
		&cobra.Command{
			Use:   "add-user <username> <password>",
			Short: "add a user to the database",
			RunE:  SimpleCommand(DatabaseAddUser),
			Args:  cobra.ExactArgs(2),
		},
	)
	return root
}

func DatabaseAddTrack(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	db, err := storage.Open(ctx, cfgFromContext(ctx))
	if err != nil {
		return err
	}

	for _, filename := range args {
		filename, err = filepath.Abs(filename)
		if err != nil {
			return err
		}

		cmd.Printf("entering: %s\n", filename)
		err = filepath.WalkDir(filename, func(path string, d fs.DirEntry, err error) error {
			cmd.Printf("attempting %s\n", path)
			if d.IsDir() {
				return nil
			}

			switch filepath.Ext(path) {
			case ".opus", ".mp3", ".flac", ".ogg":
			default:
				cmd.Printf("skipping %s not an audio file\n", path)
				return nil
			}

			return addSingleTrack(ctx, cmd, db, path)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func addSingleTrack(ctx context.Context, cmd *cobra.Command, db radio.StorageService, filename string) error {
	info, err := audio.ProbeText(ctx, filename)
	if err != nil {
		return err
	}

	// as fallback we use the filename as '{artist} - {title}'
	fn := filepath.Base(filename)
	fn = strings.TrimSuffix(fn, filepath.Ext(fn))
	artist, title, _ := strings.Cut(fn, " - ")
	if info.Title == "" {
		info.Title = title
	}
	if info.Artist == "" {
		info.Artist = artist
	}

	track := radio.DatabaseTrack{
		TrackID:    0,
		Artist:     info.Artist,
		Title:      info.Title,
		Album:      info.Album,
		FilePath:   filename,
		Tags:       "testfile",
		Acceptor:   "command-line-interface",
		LastEditor: "command-line-interface",
		Usable:     true,
	}

	song := radio.Song{
		Length:        info.Duration,
		DatabaseTrack: &track,
	}
	song.Hydrate()

	id, err := db.Track(ctx).Insert(song)
	if err != nil && !strings.Contains(err.Error(), "Duplicate") {
		return err
	}
	cmd.Printf("successfully added %s (ID: %d)\n", song.Metadata, id)
	return nil
}

func DatabaseAddUser(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	db, err := storage.Open(ctx, cfgFromContext(ctx))
	if err != nil {
		return err
	}

	name, passwd := args[0], args[1]

	u := db.User(ctx)

	// only allow adding a user this way if it doesn't exist yet
	if user, err := u.Get(name); user != nil {
		if err != nil {
			return errors.E(err, "failed to check for existing user")
		}
		return errors.E("user already exists")
	}

	hash, err := radio.GenerateHashFromPassword(passwd)
	if err != nil {
		return err
	}
	user := radio.User{
		Username: name,
		Password: string(hash),
		UserPermissions: radio.UserPermissions{
			radio.PermActive: struct{}{},
			radio.PermDev:    struct{}{},
		},
	}

	_, err = u.Create(user)
	if err != nil {
		return err
	}

	cmd.Printf("successfully added user %s\n", name)
	return nil
}

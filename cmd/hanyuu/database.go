package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/google/subcommands"
	"github.com/rs/zerolog"
)

type databaseCmd struct {
	flags *flag.FlagSet
}

func (d databaseCmd) Name() string {
	return "database"
}

func (d databaseCmd) Synopsis() string {
	return "database allows you to do some basic database mutations"
}

func (d databaseCmd) Usage() string {
	return "database"
}

func (d *databaseCmd) SetFlags(f *flag.FlagSet) {

}

func (d *databaseCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...any) subcommands.ExitStatus {
	d.flags = f
	cmder := subcommands.NewCommander(f, path.Base(os.Args[0])+" database")
	cmder.Register(cmder.HelpCommand(), "")
	cmder.Register(cmder.CommandsCommand(), "")
	cmder.Register(cmd{
		name:     "add-tracks",
		synopsis: "add a songtrack to the database",
		usage: `add-tracks <filename>:
		add the given file to the database
		`,
		execute: withConfig(d.addTrack),
	}, "")
	cmder.Register(cmd{
		name:     "add-user",
		synopsis: "add a user to the database",
		usage: `add-user <username> <password>:
		add a user to the database
		`,
		execute: withConfig(d.addUser),
	}, "")

	zerolog.Ctx(ctx).UpdateContext(func(zc zerolog.Context) zerolog.Context {
		return zc.Str("service", d.Name())
	})

	return cmder.Execute(ctx, args...)
}

func (d databaseCmd) addTrack(ctx context.Context, cfg config.Config) error {
	db, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}
	_ = db

	if len(d.flags.Args()) < 2 {
		return errors.E(errors.InvalidArgument, "no filename given")
	}

	for _, filename := range d.flags.Args()[1:] {
		filename, err = filepath.Abs(filename)
		if err != nil {
			return err
		}
		// try get information from ffprobe
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
		fmt.Printf("successfully added %s (ID: %d)\n", song.Metadata, id)
	}
	return nil
}

func (d databaseCmd) addUser(ctx context.Context, cfg config.Config) error {
	db, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	name := d.flags.Arg(1)
	if name == "" {
		return errors.E("username can't be empty")
	}
	passwd := d.flags.Arg(2)
	if passwd == "" {
		return errors.E("password can't be empty")
	}

	u := db.User(ctx)

	// only allow adding a user this way if it doesn't exist yet
	_, err = u.Get(name)
	if err != nil && !errors.Is(errors.UserUnknown, err) {
		fmt.Println("user already exists")
		return err
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
			radio.PermAdmin:  struct{}{},
		},
	}

	_, err = u.Create(user)
	if err != nil {
		return err
	}

	fmt.Println("successfully added user")
	return nil
}

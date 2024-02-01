package main

import (
	"context"
	"flag"
	"os"
	"path"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search/elastic"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/google/subcommands"
)

type elasticCmd struct {
}

func (e elasticCmd) Name() string {
	return "elastic"
}

func (e elasticCmd) Synopsis() string {
	return "helper to deal with elasticsearch service"
}

func (e elasticCmd) Usage() string {
	return "elastic"
}

func (e elasticCmd) SetFlags(*flag.FlagSet) {}

func (e elasticCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	cmder := subcommands.NewCommander(f, path.Base(os.Args[0])+" elastic")
	cmder.Register(cmder.HelpCommand(), "")
	cmder.Register(cmder.CommandsCommand(), "")

	cmder.Register(cmd{
		name:     "delete-index",
		synopsis: "deletes all indices created before",
		usage: `delete-index:
		delete all indices created by 'create-index'
		`,
		execute: withConfig(e.deleteIndex),
	}, "")
	cmder.Register(cmd{
		name:     "create-index",
		synopsis: "create all indices required",
		usage: `create-index:
		create all indices required but does not fill them with data
		`,
		execute: withConfig(e.createIndex),
	}, "")
	cmder.Register(cmd{
		name:     "index-songs",
		synopsis: "index all songs in the database",
		usage: `index-songs:
		fill the song search index with all songs in the database
		`,
		execute: withConfig(e.indexSongs),
	}, "")
	return cmder.Execute(ctx, args...)
}

func (e elasticCmd) createIndex(ctx context.Context, cfg config.Config) error {
	s, err := elastic.NewElasticSearchService(cfg)
	if err != nil {
		return err
	}

	return s.CreateIndex(ctx)
}

func (e elasticCmd) deleteIndex(ctx context.Context, cfg config.Config) error {
	s, err := elastic.NewElasticSearchService(cfg)
	if err != nil {
		return err
	}
	return s.DeleteIndex(ctx)
}

func (e elasticCmd) indexSongs(ctx context.Context, cfg config.Config) error {
	s, err := elastic.NewElasticSearchService(cfg)
	if err != nil {
		return err
	}

	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	songs, err := store.Track(ctx).All()
	if err != nil {
		return err
	}

	return s.Update(ctx, songs...)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/ircbot"
	"github.com/R-a-dio/valkyrie/manager"
	"github.com/google/subcommands"
)

type cmd struct {
	name     string
	synopsis string
	usage    string
	setFlags func(*flag.FlagSet)
	execute  func(context.Context, config.Config) error
}

func (c cmd) Name() string     { return c.name }
func (c cmd) Synopsis() string { return c.synopsis }
func (c cmd) Usage() string    { return c.usage }
func (c cmd) SetFlags(f *flag.FlagSet) {
	if c.setFlags != nil {
		c.setFlags(f)
	}
}
func (c cmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	cfg, err := config.LoadFile(configFile, configEnvFile)
	if err != nil {
		fmt.Println(err)
		return subcommands.ExitFailure
	}

	err = c.execute(ctx, cfg)
	if err != nil {
		fmt.Println(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

var versionCmd = cmd{
	name:     "version",
	synopsis: "display version information of executable.",
	usage: `version:
	display version information of executable.`,
	execute: printVersion,
}

func printVersion(context.Context, config.Config) error {
	/* uncomment when go version 1.12 lands
	if info, ok := debug.ReadBuildInfo(); ok {
		fmt.Printf("%s %s\n", info.Path, info.Main.Version)
		for _, mod := range info.Deps {
			fmt.Printf("\t%s %s\n", mod.Path, mod.Version)
		}
	}
	*/
	fmt.Printf("%s %s\n", "valkyrie", "(devel)")
	return nil
}

// configEnvFile will be resolved to the environment variable given here
var configEnvFile = "HANYUU_CONFIG"

// configFile will be filled with the -config flag value
var configFile string

var configCmd = cmd{
	name:     "config",
	synopsis: "display current configuration.",
	usage: `config:
	display current configuration.
	`,
	execute: printConfig,
}

func printConfig(_ context.Context, cfg config.Config) error {
	return cfg.Save(os.Stdout)
}

// implements cmd for .../valkyrie/manager
var managerCmd = cmd{
	name:     "manager",
	synopsis: "manages shared state between the different parts.",
	usage: `manager:
	manages shared state between the different parts.
	`,
	execute: manager.Execute,
}

// implements cmd for .../valkyrie/ircbot
var ircCmd = cmd{
	name:     "irc",
	synopsis: "run the IRC bot.",
	usage: `irc:
	run the IRC bot.
	`,
	execute: ircbot.Execute,
}

func main() {
	// setup configuration file as top-level flag
	flag.StringVar(&configFile, "config", "hanyuu.toml", "filepath to configuration file")
	// add all our top-level flags as important flags to subcommands
	flag.VisitAll(func(f *flag.Flag) {
		subcommands.ImportantFlag(f.Name)
	})
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(versionCmd, "")
	subcommands.Register(configCmd, "")
	// streamerCmd is registered in streamer.go to avoid mandatory inclusion, since it
	// depends on a C library (libmp3lame).
	// 		subcommands.Register(streamerCmd, "")
	subcommands.Register(managerCmd, "")
	subcommands.Register(ircCmd, "")

	subcommands.Register(listenerLogCmd, "jobs")
	subcommands.Register(requestCountCmd, "jobs")

	flag.Parse()
	configEnvFile = os.Getenv(configEnvFile)

	os.Exit(int(subcommands.Execute(context.TODO())))
}

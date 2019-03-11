# valkyrie
Repository of rebirth

Installation
=====

`git clone https://github.com/R-a-dio/valkyrie.git` into location of your choosing because we use the new go modules that don't require a `GOPATH`. To avoid weird tooling issues it's best to completely avoid your `GOPATH` when working with modules so don't clone it into your `GOPATH`.

Required
-----
- Go version 1.12+
- MySQL/MariaDB

Optional
-----
for work and running of `streamer/`
- ffmpeg
- ffprobe
- libmp3lame-dev

for work in `rpc/`
- [twirp](https://twitchtv.github.io/twirp/docs/install.html)

Building
=====

The project currently builds into a single executable, located in `cmd/hanyuu` run `go build` in there to acquire an executable; If you want to exclude the streamer for lack of dependencies you can run `go build -tags=nostreamer` to exclude it from building.

Configuration
-----

an example configuration file is included as `example.toml`. Other documentation on valid configuration values are located in `config/config.go`. The executable looks for a configuration file in multiple locations:
- the current working directory named `hanyuu.toml`
- the flag `-config` given to the executable
- the environment variable `HANYUU_CONFIG` which can either be a relative or absolute path

You can also run `hanyuu config` to see what the currently loaded configuration looks like, the output is a valid TOML file so can also be piped into a file if so desired

Before you commit
-----

If you've edited `rpc/radio.proto` or added a migration file under `migrations/` you should run `go generate` before you commit.
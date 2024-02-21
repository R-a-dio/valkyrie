# valkyrie
[![Go Reference](https://pkg.go.dev/badge/github.com/R-a-dio/valkyrie.svg)](https://pkg.go.dev/github.com/R-a-dio/valkyrie)
![Test](https://github.com/github/R-a-dio/valkyrie/actions/workflows/test.yml/badge.svg)
![Staticcheck](https://github.com/R-a-dio/valkyrie/actions/workflows/staticcheck.yml/badge.svg)

Repository of rebirth

Installation
=====

`git clone https://github.com/R-a-dio/valkyrie.git`

Required
-----
- Go version 1.21+
- MySQL/MariaDB

Optional
-----
for work and running of `streamer/`
- ffmpeg
- ffprobe
- libmp3lame-dev

for work in `rpc/` and running `go generate`
- [protoc](https://github.com/protocolbuffers/protobuf#protobuf-compiler-installation)
- `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`
- `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`
- `go install github.com/matryer/moq@latest`

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
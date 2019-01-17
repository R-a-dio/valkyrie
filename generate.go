package radio

//go:generate protoc --twirp_out=paths=source_relative:. --go_out=paths=source_relative:. rpc/irc/ircbot.proto
//go:generate protoc --twirp_out=paths=source_relative:. --go_out=paths=source_relative:. rpc/streamer/streamer.proto
//go:generate protoc --twirp_out=paths=source_relative:. --go_out=paths=source_relative:. rpc/manager/manager.proto

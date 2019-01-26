package radio

//go:generate protoc --twirp_out=paths=source_relative:. --go_out=paths=source_relative:. rpc/streamer.proto rpc/manager.proto rpc/announcer.proto

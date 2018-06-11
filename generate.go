package valkyrie

//go:generate protoc --twirp_out=$GOPATH/src --go_out=$GOPATH/src rpc/irc/ircbot.proto
//go:generate protoc --twirp_out=$GOPATH/src --go_out=$GOPATH/src rpc/streamer/streamer.proto
//go:generate protoc --twirp_out=$GOPATH/src --go_out=$GOPATH/src rpc/manager/manager.proto

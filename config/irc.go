package config

// DefaultIRC contains the default values of the irc bot configuration
var DefaultIRC = IRC{
	Addr: ":4550",
}

// IRC contains all the fields only relevant to the irc bot
type IRC struct {
	// Addr is the address for the HTTP API
	Addr string
	// Server is the address of the irc server to connect to
	Server string
	// Nick is the nickname to use
	Nick string
	// NickPassword is the nickserv password if any
	NickPassword string
	// Channels is the channels to join
	Channels []string
	// AllowFlood determines if flood protection is off or on
	AllowFlood bool
}

package ircbot

import (
	"fmt"
	"strings"

	"github.com/lrstanley/girc"
)

// parseCommand is a PRIVMSG handler
// that runs functions based on IRC commands
func parseCommand(c *girc.Client, e girc.Event) {
	// trims white space at beginning and end
	msg := strings.TrimSpace(e.Trailing)

	// switch for commands that do not have multiple versions
	switch msg {
	case ".fave":
		fmt.Println(e.Source.Name, "favorited the now playing song")
	case ".q":
		fmt.Println("current queue is nothing idiot")
	case ".q l":
		fmt.Println("current queue length is 0")
	case ".kill":
		fmt.Println("killing current DJ")
	}

	// random
	if msg == ".random fave" || msg == ".ra f" {
		fmt.Println("requesting favorite of", e.Source.Name)
	} else if msg == ".random" || msg == ".ra" {
		fmt.Println("requesting random")
	}
	if msg != ".random fave" && msg != ".random" && msg != ".ra f" && msg != ".ra" {
		if strings.HasPrefix(msg, ".random") || strings.HasPrefix(msg, ".ra") {
			query := strings.SplitN(msg, " ", 3)
			if query[1] == "fave" || query[1] == "f" {
				fmt.Println("requesting random fave of", query[2], query)
			} else {
				fmt.Println("requesting random song with query", query[1], query)
			}
		}
	}
}

package manager

import (
	"fmt"
	"os"

	"github.com/R-a-dio/valkyrie/config"
)

// State is a global-type application state, and is passed around all loaded
// components as a shared root
type State struct {
	config.AtomicGlobal
}

// NewState initializes a state struct with all the required items
func NewState(configPath string) (*State, error) {
	var s State

	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fmt.Println("startup: loading configuration")
	s.AtomicGlobal, err = config.LoadAtomic(f)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

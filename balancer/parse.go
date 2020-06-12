package balancer

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/R-a-dio/valkyrie/errors"
)

var (
	errNoListeners = fmt.Errorf("could not find listeners")
	rgx            = regexp.MustCompile(`Current Listeners: (\d+)`)
)

func parsexml(x []byte) (int, error) {
	const op errors.Op = "balancer/parsexml"

	res := rgx.FindSubmatch(x)
	if len(res) == 0 {
		return -1, errors.E(op, errNoListeners)
	}
	listeners, err := strconv.Atoi(string(res[1]))
	if err != nil {
		return -1, errors.E(op, err)
	}
	return listeners, nil

}

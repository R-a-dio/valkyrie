package balancer

import (
	"fmt"
	"regexp"
	"strconv"
)

func parsexml(x []byte) (int, error) {
	rgx := regexp.MustCompile(`Current Listeners: (\d+)`)
	res := rgx.FindSubmatch(x)
	if len(res) == 0 {
		return -1, fmt.Errorf("parsexml: could not find listeners, broken XML?")
	}
	listeners, err := strconv.Atoi(string(res[1]))
	if err != nil {
		return -1, fmt.Errorf("parsexml: error converting string to int %w", err)
	}
	return listeners, nil

}

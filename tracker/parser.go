package tracker

import (
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/bobg/htree"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	expectedColumns = []string{"ip", "sec. connected", "user agent", "action"}
)

func ParseListClients(r io.Reader) ([]radio.Listener, error) {
	root, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var columns []string
	var body []radio.Listener

	// find our table head, this is mostly done so we can sanity check that
	// the icecast page hasn't changed format from what we expect here
	tableHead := htree.FindEl(root, func(n *html.Node) bool {
		return n.DataAtom == atom.Thead
	})
	if tableHead == nil {
		return nil, fmt.Errorf("missing <thead> in file")
	}

	// find the table columns in the table head, the head should only have one
	// <tr> so this should be all the defined table columns
	err = htree.FindAllChildEls(tableHead, func(n *html.Node) bool {
		return n.DataAtom == atom.Td
	}, storeText(&columns, true))
	if err != nil {
		return nil, err
	}

	// here we do our sanity check on the icecast table header
	if len(expectedColumns) != len(columns) {
		// warning: invalid column count found in listclients
	}
	if slices.Compare(expectedColumns, columns) != 0 {
		// warning: different column names in listclients
	}

	// now we continue with the table body, this will have one row per
	// active listener
	tableBody := htree.FindEl(root, func(n *html.Node) bool {
		return n.DataAtom == atom.Tbody
	})
	if tableBody == nil {
		return nil, fmt.Errorf("missing <tbody> in file")
	}

	// find all the table rows
	err = htree.FindAllChildEls(tableBody, func(n *html.Node) bool {
		return n.DataAtom == atom.Tr
	}, func(n *html.Node) error {
		var data []string

		// find all the columns in the row
		err = htree.FindAllChildEls(n, func(n *html.Node) bool {
			return n.DataAtom == atom.Td
		}, storeText(&data, false))
		if err != nil {
			return err
		}

		// sanity check that we got 4 columns worth of data
		if len(data) != 4 {
			return nil
		}

		// parse our listener id from the final column
		id, err := radio.ParseListenerClientID(data[3])
		if err != nil {
			return err
		}

		// parse our start time by subtracting the connection duration
		// from now
		start := time.Now()
		div, err := strconv.ParseUint(data[1], 10, 64)
		if err != nil {
			return err
		}
		start = start.Add(-time.Duration(div) * time.Second)

		// construct the listener with the above data and the leftover text
		body = append(body, radio.Listener{
			ID:        id,
			UserAgent: data[2],
			IP:        data[0],
			Start:     start,
		})
		return nil
	})

	return body, nil
}

// storeText converts the nodes given to text with htree.Text and
// stores the result in `out` after trimming whitespace
//
// if `lower` is true it runs strings.ToLower as well.
//
// as a special case, if the text == "Kick" and we're on the fourth column
// we run getKickID on the node.
func storeText(out *[]string, lower bool) func(*html.Node) error {
	return func(n *html.Node) error {
		text, err := htree.Text(n)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if lower {
			text = strings.ToLower(text)
		}
		if len(*out) == 3 && text == "Kick" {
			// if we're on the expected kick column, convert the
			// text into the ID hidden inside the <a> instead
			text = getKickID(n)
		}

		*out = append(*out, text)
		return nil
	}
}

// getKickID finds the id=? value in the icecast admin pages Kick column.
// n should be the <td> node and is expected to look like this in html form:
//
// <td>
// <a href="killclient.xsl?mount=/main.mp3&amp;id=30">Kick</a>
// </td>
//
// where the expected return value here would be "30"
func getKickID(n *html.Node) string {
	// find the <a> inside the node
	a := htree.Find(n, func(n *html.Node) bool {
		return n.DataAtom == atom.A
	})
	if a == nil {
		return ""
	}

	// then grab the href attribute
	uri := htree.ElAttr(a, atom.Href.String())

	// this should be a valid url, so parse it
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}

	// and then only grab the id parameter
	rawid := u.Query().Get("id")
	if rawid == "" {
		return ""
	}

	return rawid
}

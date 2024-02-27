package shared

import (
	"bytes"
	"reflect"
	"strconv"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func NoBlockQuoteParser() parser.Parser {
	var found bool
	adjustedBlockParsers := parser.DefaultBlockParsers()
	for i, p := range adjustedBlockParsers {
		pt := reflect.TypeOf(p.Value)
		if pt.Kind() == reflect.Pointer {
			pt = pt.Elem()
		}
		if pt.Name() == "blockquoteParser" {
			adjustedBlockParsers = append(adjustedBlockParsers[:i], adjustedBlockParsers[i+1:]...)
			found = true
		}
	}

	if !found {
		panic("block quote parser wasn't present")
	}

	return parser.NewParser(
		parser.WithBlockParsers(adjustedBlockParsers...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

type MemeQuoteParser struct{}

var _ parser.InlineParser = (*MemeQuoteParser)(nil)

var (
	linkMeme  = []byte(">>")
	greenMeme = []byte(">")
)

func (p *MemeQuoteParser) Trigger() []byte {
	return []byte{'>'}
}

func (p *MemeQuoteParser) Parse(parent ast.Node, reader text.Reader, pc parser.Context) ast.Node {
	line, seg := reader.PeekLine()

	if bytes.HasPrefix(line, linkMeme) {
		// we have >> which should get us a link to another comment
		// find the next whitespace to terminate on
		stop := bytes.IndexFunc(line, unicode.IsSpace)
		if stop < 0 {
			// no whitespace left?
			return nil
		}

		// grab the number, if it is one
		shouldNumber := line[2:stop]
		number, err := strconv.ParseInt(string(shouldNumber), 10, 64)
		if err != nil {
			// it wasn't a number, leave it alone
			return nil
		} else if number < 0 {
			// or if it's below zero, we don't have any comment IDs below zero
			return nil
		}

		// grab our little meme
		seg = text.NewSegment(seg.Start, seg.Start+stop)
		// and construct a link out of it
		link := ast.NewLink()
		// we can use shouldNumber here since we made sure it is an actual number above
		link.Destination = append([]byte("#"), shouldNumber...)
		// add the actual text as child of the link
		link.AppendChild(link, ast.NewTextSegment(seg))

		reader.Advance(stop)
		return link
	}

	if bytes.HasPrefix(line, greenMeme) {
		parent.SetAttributeString("class", []byte("green-text"))
		return nil
	}

	return nil
}

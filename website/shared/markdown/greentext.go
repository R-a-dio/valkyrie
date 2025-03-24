package markdown

import (
	"bytes"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	goldutil "github.com/yuin/goldmark/util"
)

func RadioMarkdownOptions(debug bool) []goldmark.Option {
	return []goldmark.Option{
		goldmark.WithParser(NoBlockQuoteParser()),
		goldmark.WithParserOptions(
			parser.WithInlineParsers(
				goldutil.Prioritized(&MemeQuoteParser{debugEnabled: debug}, 1),
			),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			renderer.WithNodeRenderers(
				goldutil.Prioritized(&MemeQuoteRenderer{}, 1),
			),
		),
	}
}

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

type MemeQuoteParser struct {
	debugEnabled bool
}

func (p *MemeQuoteParser) debug(s string, args ...any) {
	if p.debugEnabled {
		log.Printf(s, args...)
	}
}

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
			p.debug("no whitespace at end of line found")
			// continue with just the whole line then
			stop = len(line)
		}

		// grab the number, if it is one
		shouldNumber := line[2:stop]
		number, err := strconv.ParseInt(string(shouldNumber), 10, 64)
		if err != nil {
			// it wasn't a number, leave it alone
			p.debug("text after >> wasn't a number")
			return nil
		} else if number < 0 {
			p.debug("number is below zero")
			// or if it's below zero, we don't have any comment IDs below zero
			return nil
		}

		// grab our little meme
		seg = text.NewSegment(seg.Start, seg.Start+stop)
		// and construct a link out of it
		link := ast.NewLink()
		// we can use shouldNumber here since we made sure it is an actual number above
		link.Destination = append([]byte("#comment-"), shouldNumber...)
		// add the actual text as child of the link
		link.AppendChild(link, ast.NewTextSegment(seg))

		reader.Advance(stop)
		return link
	}

	// TODO: implement this correctly
	if bytes.HasPrefix(line, greenMeme) && reader.LineOffset() == 0 {
		// if there is a
		stop := len(line)
		if tmp := bytes.IndexByte(line, '\n'); tmp > 0 {
			// don't cut off the newline if it exists
			stop = tmp
		}

		seg = text.NewSegment(seg.Start, seg.Start+stop)

		green := &Node{}
		green.AppendChild(green, ast.NewTextSegment(seg))
		reader.Advance(stop)
		return green
	}

	return nil
}

type MemeQuoteRenderer struct{}

var Kind = ast.NewNodeKind("greentext")

type Node struct {
	ast.BaseInline
}

func (Node) Kind() ast.NodeKind {
	return Kind
}

func (n *Node) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

var _ ast.Node = (*Node)(nil)

func (r *MemeQuoteRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(Kind, r.Render)
}

func (r *MemeQuoteRenderer) Render(w goldutil.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	_, ok := node.(*Node)
	if !ok {
		return ast.WalkStop, fmt.Errorf("unexpected node %T, expected 'mememark.Node'", node)
	}

	if entering {
		_, _ = w.WriteString(`<span class="green-text">`)
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`</span>`)

	return ast.WalkContinue, nil
}

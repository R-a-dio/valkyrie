package markdown

import (
	"fmt"
	"log"
	"time"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	goldutil "github.com/yuin/goldmark/util"
)

type TimeParser struct {
	debugEnabled bool
}

func (p *TimeParser) debug(s string, args ...any) {
	if p.debugEnabled {
		log.Printf(s, args...)
	}
}

var _ parser.InlineParser = (*TimeParser)(nil)

const (
	timeStart = '{'
	timeEnd   = '}'
)

func (p *TimeParser) Trigger() []byte {
	return []byte{timeStart}
}

func (p *TimeParser) Parse(parent ast.Node, reader text.Reader, pc parser.Context) ast.Node {
	if reader.Peek() != timeStart {
		return nil
	}

	// eat our beginning
	reader.Advance(1)
	_, seg := reader.Position()
	var i int
	rs := []rune{}
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return nil
		}
		if r == timeEnd {
			break
		} else if unicode.IsSpace(r) { // failsafe
			p.debug("space encountered")
			return nil
		}
		rs = append(rs, r)
		i++
	}

	seg = text.NewSegment(seg.Start, seg.Start+i)

	tn := &tNode{}
	var err error
	tn.t, err = time.Parse(time.RFC3339, string(rs))
	if err != nil {
		p.debug("error parsing time")
		return nil
	}
	tn.AppendChild(tn, ast.NewTextSegment(seg))
	return tn
}

type TimeRenderer struct{}

var tKind = ast.NewNodeKind("ltime")

type tNode struct {
	ast.BaseInline
	t time.Time
}

func (tNode) Kind() ast.NodeKind {
	return tKind
}

func (n *tNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

var _ ast.Node = (*tNode)(nil)

func (r *TimeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(tKind, r.Render)
}

func (r *TimeRenderer) Render(w goldutil.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	tn, ok := node.(*tNode)
	if !ok {
		return ast.WalkStop, fmt.Errorf("unexpected node %T, expected 'time.Node'", node)
	}

	if entering {
		fmt.Fprintf(w, `<time class="ltime" datetime="%d">`, tn.t.Unix())
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`</time>`)

	return ast.WalkContinue, nil
}

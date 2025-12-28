package markdown

import (
	"fmt"
	"log"
	"strings"
	"time"

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
	durSep    = "|"
)

func (p *TimeParser) Trigger() []byte {
	return []byte{timeStart}
}

func (p *TimeParser) Parse(parent ast.Node, reader text.Reader, pc parser.Context) ast.Node {
	// eat our beginning
	reader.Advance(1)
	_, seg := reader.Position()
	rs := []rune{}
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return nil
		}
		if r == timeEnd {
			break
		}
		rs = append(rs, r)
	}

	ts, dur, hasDuration := strings.Cut(string(rs), durSep)

	seg = text.NewSegment(seg.Start, seg.Start+len(ts))

	tn := &tNode{}
	var err error
	tn.t, err = time.Parse(time.RFC822Z, ts)
	if err != nil {
		p.debug("error parsing time")
		return nil
	}
	if hasDuration {
		tn.d, err = time.ParseDuration(dur)
		if err != nil {
			p.debug("error parsing duration")
			return nil
		}
	}
	tn.AppendChild(tn, ast.NewTextSegment(seg))
	return tn
}

type TimeRenderer struct{}

var tKind = ast.NewNodeKind("ltime")

type tNode struct {
	ast.BaseInline
	t time.Time
	d time.Duration
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
		fmt.Fprintf(w, `<time datetime="%d" data-dur="%d" data-type="local">`, tn.t.Unix(), tn.d.Milliseconds())
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`</time>`)

	return ast.WalkContinue, nil
}

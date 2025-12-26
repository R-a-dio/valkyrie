package markdown

import (
	"fmt"
	"log"
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
	durStart  = '|'
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
	var indur bool
	timeRs := []rune{}
	durRs := []rune{}
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return nil
		}
		if r == timeEnd {
			break
		} else if r == durStart { // allow a duration, don't include it in output text
			indur = true
			continue
		} else if r == '\n' { // failsafe
			p.debug("newline encountered")
			return nil
		}
		if indur {
			durRs = append(durRs, r)
			continue
		}
		timeRs = append(timeRs, r)
		i++
	}

	seg = text.NewSegment(seg.Start, seg.Start+i)

	tn := &tNode{}
	var err error
	tn.t, err = time.Parse(time.RFC822Z, string(timeRs))
	if err != nil {
		p.debug("error parsing time")
		return nil
	}
	if len(durRs) != 0 {
		tn.d, err = time.ParseDuration(string(durRs))
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
		fmt.Fprintf(w, `<time class="ltime" datetime="%d" data-dur="%d">`, tn.t.Unix(), tn.d.Milliseconds())
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`</time>`)

	return ast.WalkContinue, nil
}

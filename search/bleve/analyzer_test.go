package bleve

import (
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/blevesearch/bleve/v2/analysis/tokenizer/character"

	"github.com/stretchr/testify/assert"
)

func FuzzTokenizer(f *testing.F) {
	tokenizer := character.NewCharacterTokenizer(func(r rune) bool { return !unicode.IsSpace(r) })
	tok := NewKagomeTokenizer(tokenizer)

	f.Fuzz(func(t *testing.T, in []byte) {
		now := time.Now()
		out := tok.Tokenize(in)
		taken := time.Since(now)
		t.Log(taken)

		for _, token := range out {
			if !assert.Equal(t, token.Term, in[token.Start:token.End]) {
				t.Log(in)
				t.Log(token.Term)
				t.Log(token)
				t.Fail()
			}
		}

		if taken > time.Second*1 {
			t.Fatal("took too long", taken, in)
		}
	})
}

var benches = []struct {
	name  string
	input string
}{
	{"rune-small", "桃色技術音楽堂"},
	{"rune-medium", "feat. Hanamori Rie Pink smoothie 桃色技術音楽堂×中路もとめ"},
	{"rune-worst-case", repeatToLength("桃色技術音楽堂", MaxQuerySize)},
	{"ascii-small", "Hikasa Youko"},
	{"ascii-medium", "Toyosaki Aki & Hikasa Youko & Satou Satomi & Kotobuki Minako & Taketatsu Ayana U&I (Studio Mix)"},
	{"ascii-worst-case", repeatToLength("Toyosaki Aki & Hikasa Youko & Satou Satomi & Kotobuki Minako & Taketatsu Ayana U&I (Studio Mix)", MaxQuerySize)},
}

func repeatToLength(s string, l int) string {
	return CutoffAtRune(strings.Repeat(s, l/len(s)+1)[:l])
}

func BenchmarkRadioAnalyzer(b *testing.B) {
	idx := newIndex(b)
	a := idx.index.Mapping().AnalyzerNamed(radioAnalyzerName)

	fn := func(s string) func(b *testing.B) {
		in := []byte(s)
		return func(b *testing.B) {
			for range b.N {
				a.Analyze(in)
			}
		}
	}

	for _, bench := range benches {
		b.Run(bench.name, fn(bench.input))
	}
}

func BenchmarkExactAnalyzer(b *testing.B) {
	idx := newIndex(b)
	a := idx.index.Mapping().AnalyzerNamed(exactAnalyzerName)

	fn := func(s string) func(b *testing.B) {
		in := []byte(s)
		return func(b *testing.B) {
			for range b.N {
				a.Analyze(in)
			}
		}
	}

	for _, bench := range benches {
		b.Run(bench.name, fn(bench.input))
	}
}

func BenchmarkTokenizer(b *testing.B) {
	tokenizer := character.NewCharacterTokenizer(IsNotSpace)
	tok := NewKagomeTokenizer(tokenizer)

	fn := func(s string) func(b *testing.B) {
		in := []byte(s)
		return func(b *testing.B) {
			for range b.N {
				tok.Tokenize(in)
			}
		}
	}

	for _, bench := range benches {
		b.Run(bench.name, fn(bench.input))
	}
}

func BenchmarkIsASCII(b *testing.B) {
	fn := func(s string) func(b *testing.B) {
		in := []byte(s)
		return func(b *testing.B) {
			for range b.N {
				IsASCII(in)
			}
		}
	}

	for _, bench := range benches {
		b.Run(bench.name, fn(bench.input))
	}
}

func BenchmarkCutoffWorstCase(b *testing.B) {
	// this benchmark just tests our worst case if someone crafted a string specifically to
	// hit the RuneError path all the way to nothing
	var i []rune
	for range MaxQuerySize {
		i = append(i, utf8.RuneError)
	}
	in := string(i)
	assert.Len(b, CutoffAtRune(in), 0)

	b.ResetTimer()
	for range b.N {
		CutoffAtRune(in)
	}
}

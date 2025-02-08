package bleve

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/token/length"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/token/ngram"
	"github.com/blevesearch/bleve/v2/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/v2/analysis/token/unique"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/character"
	"github.com/blevesearch/bleve/v2/registry"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"github.com/robpike/nihongo"
)

const NgramFilterMin = 2
const NgramFilterMax = 3

var _ analysis.Analyzer = new(multiAnalyzer)

type PrefilterFn func(in []byte) (out []byte)

type multiAnalyzer struct {
	prefilter func(in []byte) (out []byte)
	analyzers []analysis.Analyzer
}

func (ma *multiAnalyzer) Analyze(text []byte) analysis.TokenStream {
	var res analysis.TokenStream

	fmt.Println(string(text))
	if ma.prefilter != nil {
		new := ma.prefilter(text)
		if !bytes.Equal(text, new) {
			res = ma.analyze(res, new)
		}
	}

	return ma.analyze(res, text)
}

func (ma *multiAnalyzer) analyze(res analysis.TokenStream, text []byte) analysis.TokenStream {
	for _, a := range ma.analyzers {
		res = append(res, a.Analyze(text)...)
	}
	return res
}

func NewMultiAnalyzer(pre PrefilterFn, a ...analysis.Analyzer) analysis.Analyzer {
	return &multiAnalyzer{
		prefilter: pre,
		analyzers: a,
	}
}

func AnalyzerConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.Analyzer, error) {
	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	normalizeFilter := unicodenorm.MustNewUnicodeNormalizeFilter(unicodenorm.NFC)

	tokenizer := character.NewCharacterTokenizer(func(r rune) bool { return !unicode.IsSpace(r) })

	// construct the japanese specific analyzer
	japanese := &analysis.DefaultAnalyzer{
		Tokenizer: NewKagomeTokenizer(tokenizer),
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			normalizeFilter,
			FilterFn(RomajiFilter),
			NgramFilter(NgramFilterMin, NgramFilterMax),
			unique.NewUniqueTermFilter(),
		},
	}

	return japanese, nil
}

func ExactAnalyzerConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.Analyzer, error) {
	tokenizer, err := cache.TokenizerNamed("unicode")
	if err != nil {
		return nil, err
	}

	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	normalizeFilter := unicodenorm.MustNewUnicodeNormalizeFilter(unicodenorm.NFC)

	// construct an exact term analyzer
	exact := &analysis.DefaultAnalyzer{
		Tokenizer: NewKagomeTokenizer(tokenizer),
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			normalizeFilter,
			length.NewLengthFilter(2, 0), // filter away single char terms
			unique.NewUniqueTermFilter(),
		},
	}
	return exact, nil
}

func init() {
	registry.RegisterAnalyzer("radio", AnalyzerConstructor)
	registry.RegisterAnalyzer("exact", ExactAnalyzerConstructor)
}

type FilterFn func(input analysis.TokenStream) analysis.TokenStream

func (fn FilterFn) Filter(input analysis.TokenStream) analysis.TokenStream {
	return fn(input)
}

func DebugFilter(prefix string) analysis.TokenFilter {
	return FilterFn(func(input analysis.TokenStream) analysis.TokenStream {
		fmt.Printf("======== %s ========\n", prefix)
		for i, token := range input {
			fmt.Printf("%d %s\n", i, token)
		}
		fmt.Printf("======== %s ========\n", prefix)
		return input
	})
}

func RomajiFilter(input analysis.TokenStream) analysis.TokenStream {
	rv := make(analysis.TokenStream, 0, len(input))

	for _, token := range input {
		// include the original token
		rv = append(rv, token)

		new := nihongo.Romaji(token.Term)
		if !bytes.Equal(new, token.Term) {
			token := analysis.Token{
				Position: token.Position,
				Start:    token.Start,
				End:      token.End,
				Type:     token.Type,
				KeyWord:  true,
				Term:     new,
			}
			rv = append(rv, &token)
		}
	}

	return rv
}

func NgramFilter(min, max int) analysis.TokenFilter {
	ngram := ngram.NewNgramFilter(min, max)

	return FilterFn(func(input analysis.TokenStream) analysis.TokenStream {
		rv := make(analysis.TokenStream, 0, len(input))

		for i, tok := range input {
			if len(tok.Term) > max {
				// add the original token if it's above max
				//rv = append(rv, tok)
			}
			// add the ngram tokens if this isn't a shingle
			if tok.Type != analysis.Shingle {
				rv = append(rv, ngram.Filter(input[i:i+1])...)
			}
		}
		return rv
	})
}

type KagomeTokenizer struct {
	kagome     *tokenizer.Tokenizer
	whitespace analysis.Tokenizer
}

func NewKagomeTokenizer(tokeniz analysis.Tokenizer) *KagomeTokenizer {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil
	}

	return &KagomeTokenizer{
		kagome:     tok,
		whitespace: tokeniz,
	}
}

// IsASCII checks if the input only contains ascii
func IsASCII(input []byte) bool {
	for _, c := range input {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func (t *KagomeTokenizer) Tokenize(input []byte) analysis.TokenStream {
	const DEBUG = false
	if len(input) < 1 {
		return nil
	}
	if IsASCII(input) { // check if we need kagome or can just use the standard whitespace tokenizer
		return t.whitespace.Tokenize(input)
	}

	var bytePos int
	var surface []byte
	var whitespaceLen int
	var rv = make(analysis.TokenStream, 0, 5)
	var tokenPos int

	appendToken := func(token *analysis.Token) {
		rv, tokenPos = append(rv, token), tokenPos+1
	}

	for _, m := range t.kagome.Analyze(string(input), tokenizer.Search) {
		if DEBUG {
			log.Println(m)
			log.Println([]byte(m.Surface))
		}
		bytePos += len(m.Surface) // add to the running byte count

		// length before we trim
		surfaceLen := len(m.Surface)
		m.Surface = strings.TrimRightFunc(m.Surface, unicode.IsSpace)
		// then calculate how much whitespace we removed
		whitespaceLen = surfaceLen - len(m.Surface)
		if len(m.Surface) < surfaceLen {
			// we removed something from the surface, add it to our running count and then mark
			// the m.Surface as empty so we enter the next condition
			surface = append(surface, m.Surface...)
			m.Surface = m.Surface[:0]
		}

		if len(m.Surface) == 0 && len(surface) > 0 {
			// we found some whitespace, emit everything we've collected in the surface
			token := &analysis.Token{
				Term:     surface,
				Position: tokenPos,
				Start:    bytePos - len(surface) - whitespaceLen,
				End:      bytePos - whitespaceLen,
				Type:     analysis.AlphaNumeric,
			}

			appendToken(token)
			surface = nil
			continue
		}

		if m.Class == tokenizer.KNOWN {
			// we hit something that the tokenizer knows, this probably means some
			// japanese text, emit whatever is in the current surface first and then
			// handle the new token
			if len(surface) > 0 {
				token := &analysis.Token{
					Term:     surface,
					Position: tokenPos,
					Start:    bytePos - len(surface) - len(m.Surface),
					End:      bytePos - len(m.Surface),
					Type:     analysis.AlphaNumeric,
				}

				appendToken(token)
				surface = nil
			}

			// now handle the KNOWN token
			token := &analysis.Token{
				Term:     []byte(m.Surface),
				Position: tokenPos,
				Start:    bytePos - len(m.Surface),
				End:      bytePos,
				Type:     analysis.Ideographic,
			}
			appendToken(token)
			continue
		}

		surface = append(surface, m.Surface...)
	}

	// end of the input, might have a strangling surface
	if len(surface) > 0 {
		token := &analysis.Token{
			Term:     surface,
			Position: tokenPos,
			Start:    bytePos - len(surface) - whitespaceLen,
			End:      bytePos - whitespaceLen,
			Type:     analysis.AlphaNumeric,
		}

		rv = append(rv, token)
	}

	/*
		fmt.Printf("%s ->	", string(input))
		for _, token := range rv {
			fmt.Printf("[%s]", string(token.Term))
		}
		fmt.Printf("\n")
	*/

	/*
		for _, token := range rv {
			fmt.Printf("TOKEN: %v\n", token)
			fmt.Println(string(input[token.Start:token.End]))
		}
	*/
	return rv
}

/*func KagomeFilter() (FilterFn, error) {
	t, err := tokenizer.New(uni.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil, err
	}

	isKanji := regexp.MustCompile(`^[\p{Han}\p{Hiragana}\p{Katakana}]+$`).Match

	return FilterFn(func(input analysis.TokenStream) analysis.TokenStream {
		rv := make(analysis.TokenStream, 0, len(input))

		fmt.Println("============================")
		for _, token := range input {
			rv = append(rv, token)

			fmt.Println(token)
			if !isKanji(token.Term) {
				continue
			}

			extras := t.Analyze(string(token.Term), tokenizer.Extended)
			merged := make([]string, 0, len(extras))
			for _, x := range extras {
				if x.Class == tokenizer.UNKNOWN {
					continue
				}
				r, ok := x.Pronunciation()
				fmt.Println(string(token.Term), r, ok)
				rom := nihongo.RomajiString(r)
				merged = append(merged, rom)
			}
			fmt.Println(strings.Join(merged, ""))
		}

		return rv
	}), nil
}*/

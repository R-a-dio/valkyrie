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
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/single"
	"github.com/blevesearch/bleve/v2/registry"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"github.com/robpike/nihongo"
)

const (
	radioAnalyzerName = "radio"
	exactAnalyzerName = "radio.exact"
	sortAnalyzerName  = "radio.sort"

	NgramFilterMin = 2
	NgramFilterMax = 3
)

func IsNotSpace(r rune) bool {
	return !unicode.IsSpace(r)
}

func SortAnalyzerConstructor(config map[string]any, cache *registry.Cache) (analysis.Analyzer, error) {
	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	normalizeFilter := unicodenorm.MustNewUnicodeNormalizeFilter(unicodenorm.NFC)

	return &analysis.DefaultAnalyzer{
		Tokenizer: single.NewSingleTokenTokenizer(),
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			normalizeFilter,
		},
	}, nil
}
func RadioAnalyzerConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.Analyzer, error) {
	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	normalizeFilter := unicodenorm.MustNewUnicodeNormalizeFilter(unicodenorm.NFC)

	tokenizer := character.NewCharacterTokenizer(IsNotSpace)

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
	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	normalizeFilter := unicodenorm.MustNewUnicodeNormalizeFilter(unicodenorm.NFC)

	tokenizer := character.NewCharacterTokenizer(IsNotSpace)

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
	registry.RegisterAnalyzer(radioAnalyzerName, RadioAnalyzerConstructor)
	registry.RegisterAnalyzer(exactAnalyzerName, ExactAnalyzerConstructor)
	registry.RegisterAnalyzer(sortAnalyzerName, SortAnalyzerConstructor)
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

func IsPunctOrSymbol(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func (t *KagomeTokenizer) Tokenize(input []byte) analysis.TokenStream {
	const DEBUG = false
	if len(input) < 1 {
		return nil
	}

	var bytePos int
	var rv = make(analysis.TokenStream, 0, 5)
	var tokenPos int

	appendToken := func(token *analysis.Token) {
		rv, tokenPos = append(rv, token), tokenPos+1
	}

	for _, wt := range t.whitespace.Tokenize(input) {
		// Check if ASCII to avoid tokenization on punctuation, and
		// save on time
		if IsASCII(wt.Term) {
			// Strip punctuation from the left and right of the term
			s := string(wt.Term)
			sl := strings.TrimLeftFunc(s, IsPunctOrSymbol)
			sr := strings.TrimRightFunc(sl, IsPunctOrSymbol)
			if len(sr) == 0 {
				// No point in adding an empty term
				continue
			}
			wt.Start += len(s) - len(sl)
			wt.End -= len(sl) - len(sr)
			wt.Term = []byte(sr)

			if DEBUG {
				log.Println(wt)
			}
			appendToken(wt)
			continue
		}

		// Kagome splits up punctuation from terms by default, so it doesn't need
		// the stripping
		bytePos = 0
		for _, m := range t.kagome.Analyze(string(wt.Term), tokenizer.Search) {
			surface := []byte(m.Surface)
			if DEBUG {
				log.Println(m)
				log.Println(surface)
			}
			class := analysis.AlphaNumeric
			if m.Class == tokenizer.KNOWN {
				// KNOWN is japanese text
				class = analysis.Ideographic
			}
			token := &analysis.Token{
				Term:     surface,
				Position: tokenPos,
				Start:    wt.Start + bytePos,
				End:      wt.Start + bytePos + len(surface),
				Type:     class,
			}
			if DEBUG {
				log.Println(token)
			}
			appendToken(token)
			bytePos += len(surface)
		}
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

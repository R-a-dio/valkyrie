package bleve

import (
	"bytes"
	"fmt"

	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/web"
	"github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/token/ngram"
	"github.com/blevesearch/bleve/v2/analysis/token/shingle"
	"github.com/blevesearch/bleve/v2/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/whitespace"
	"github.com/blevesearch/bleve/v2/registry"
	"github.com/robpike/nihongo"
)

func AnalyzerConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.Analyzer, error) {
	tokenizer, err := cache.TokenizerNamed(web.Name)
	if err != nil {
		return nil, err
	}

	cjkWidth, err := cache.TokenFilterNamed(cjk.WidthName)
	if err != nil {
		return nil, err
	}

	cjkFilter, err := cache.TokenFilterNamed(cjk.BigramName)
	if err != nil {
		return nil, err
	}
	_ = cjkFilter

	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	rv := analysis.DefaultAnalyzer{
		Tokenizer: tokenizer,
		TokenFilters: []analysis.TokenFilter{
			cjkWidth,
			shingle.NewShingleFilter(2, 4, true, " ", "_"),
			FilterFn(RomajiFilter),
			toLowerFilter,
			cjkFilter,
			unicodenorm.MustNewUnicodeNormalizeFilter(unicodenorm.NFC),
			NgramFilter(2, 3),
		},
	}
	return &rv, nil
}

func QueryAnalyzerConstructor(config map[string]any, cache *registry.Cache) (analysis.Analyzer, error) {
	tokenizer, err := cache.TokenizerNamed(whitespace.Name)
	if err != nil {
		return nil, err
	}

	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}

	rv := analysis.DefaultAnalyzer{
		Tokenizer: tokenizer,
		TokenFilters: []analysis.TokenFilter{
			FilterFn(RomajiFilter),
			toLowerFilter,
		},
	}

	return &rv, nil
}

func init() {
	registry.RegisterAnalyzer("radio", AnalyzerConstructor)
	registry.RegisterAnalyzer("radio-query", QueryAnalyzerConstructor)
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
				rv = append(rv, tok)
			}
			// add the ngram tokens if this isn't a shingle
			if tok.Type != analysis.Shingle {
				rv = append(rv, ngram.Filter(input[i:i+1])...)
			}
		}
		return rv
	})
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

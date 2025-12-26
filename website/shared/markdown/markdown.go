package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	goldutil "github.com/yuin/goldmark/util"
)

func RadioMarkdownOptions(debug bool) []goldmark.Option {
	return []goldmark.Option{
		goldmark.WithParser(NoBlockQuoteParser()),
		goldmark.WithParserOptions(
			parser.WithInlineParsers(
				goldutil.Prioritized(&MemeQuoteParser{debugEnabled: debug}, 1),
				goldutil.Prioritized(&TimeParser{debugEnabled: debug}, 1),
			),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			renderer.WithNodeRenderers(
				goldutil.Prioritized(&MemeQuoteRenderer{}, 1),
				goldutil.Prioritized(&TimeRenderer{}, 1),
			),
		),
	}
}

package templates

import (
	"encoding/json"
	"html/template"

	"github.com/R-a-dio/valkyrie/errors"
)

func TemplateFuncs() template.FuncMap {
	return fnMap
}

var fnMap = map[string]any{
	"printjson":    PrintJSON,
	"safeHTML":     SafeHTML,
	"safeHTMLAttr": SafeHTMLAttr,
}

func PrintJSON(v any) (template.HTML, error) {
	b, err := json.MarshalIndent(v, "", "\t")
	return template.HTML("<pre>" + string(b) + "</pre>"), err
}

func SafeHTML(v any) (template.HTML, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.E(errors.InvalidArgument)
	}
	return template.HTML(s), nil
}

func SafeHTMLAttr(v any) (template.HTMLAttr, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.E(errors.InvalidArgument)
	}
	return template.HTMLAttr(s), nil
}

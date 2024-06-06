package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"reflect"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

func TemplateFuncs() template.FuncMap {
	return fnMap
}

var fnMap = map[string]any{
	"printjson":                   PrintJSON,
	"safeHTML":                    SafeHTML,
	"safeHTMLAttr":                SafeHTMLAttr,
	"Until":                       time.Until,
	"Since":                       time.Since,
	"Now":                         time.Now,
	"ToSecond":                    func(d time.Duration) int64 { return int64(d.Seconds()) },
	"TimeagoDuration":             TimeagoDuration,
	"PrettyDuration":              PrettyDuration,
	"AbsoluteDate":                AbsoluteDate,
	"HumanDuration":               HumanDuration,
	"MediaDuration":               MediaDuration,
	"Div":                         func(a, b any) int64 { return castInt64(a) / castInt64(b) },
	"Sub":                         func(a, b any) int64 { return castInt64(a) / castInt64(b) },
	"CalculateSubmissionCooldown": radio.CalculateSubmissionCooldown,
	"AllUserPermissions":          radio.AllUserPermissions,
	"HasField":                    HasField,
	"SongPair":                    SongPair,
}

type SongPairing struct {
	*radio.Song
	Data any
}

func SongPair(song radio.Song, data any) SongPairing {
	return SongPairing{
		Song: &song,
		Data: data,
	}
}

func castInt64(a any) int64 {
	switch a := a.(type) {
	case int:
		return int64(a)
	case int16:
		return int64(a)
	case int32:
		return int64(a)
	case int64:
		return a
	case uint:
		return int64(a)
	case uint16:
		return int64(a)
	case uint32:
		return int64(a)
	case uint64:
		return int64(a)
	}
	panic("invalid type in castInt64")
}

func HasField(v any, name string) bool {
	rv := reflect.ValueOf(v)
	rv = reflect.Indirect(rv)
	return rv.FieldByName(name).IsValid()
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

func TimeagoDuration(d time.Duration) string {
	if d > 0 { // future duration
		if d <= time.Minute {
			return "in less than a min"
		}
		if d < time.Minute*2 {
			return fmt.Sprintf("in %.0f min", d.Minutes())
		}
		return fmt.Sprintf("in %.0f mins", d.Minutes())
	} else { // past duration
		d = d.Abs()
		if d <= time.Minute {
			return "less than a min ago"
		}
		if d < time.Minute*2 {
			return fmt.Sprintf("%.0f min ago", d.Minutes())
		}
		return fmt.Sprintf("%.0f mins ago", d.Minutes())
	}
}

func PrettyDuration(d time.Duration) string {
	if d > 0 { // future duration
		if d <= time.Minute {
			return "in less than a minute"
		}
		if d < time.Minute*2 {
			return fmt.Sprintf("in %.0f minute", d.Minutes())
		}
		return fmt.Sprintf("in %.0f minutes", d.Minutes())
	} else { // past duration
		d = d.Abs()
		if d <= time.Minute {
			return "less than a minute ago"
		}
		if d < time.Minute*2 {
			return fmt.Sprintf("%.0f minute ago", d.Minutes())
		}
		return fmt.Sprintf("%.0f minutes ago", d.Minutes())
	}
}

func AbsoluteDate(t time.Time) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if t.Before(today) {
		return t.Format("2006-01-02 15:04:05 MST")
	}
	return t.Format("15:04:05 MST")
}

func HumanDuration(d time.Duration) string {
	const day = time.Hour * 24

	d = d.Truncate(time.Second)

	days := d / day
	if days > 0 {
		return fmt.Sprintf("%dd%s", days, d%day)
	}
	return d.String()
}

func MediaDuration(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", d/time.Minute, d%time.Minute/time.Second)
}

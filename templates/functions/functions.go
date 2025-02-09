package functions

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/buildinfo"
	"github.com/dustin/go-humanize"
)

func NewStatefulFunctions(cfg config.Config, status util.StreamValuer[radio.Status]) *StatefulFuncs {
	return &StatefulFuncs{
		musicPath: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().MusicPath
		}),
		status: status,
	}
}

type StatefulFuncs struct {
	musicPath func() string
	status    util.StreamValuer[radio.Status]
}

func (sf *StatefulFuncs) Status() radio.Status {
	return sf.status.Latest()
}

func (sf *StatefulFuncs) SongFileSize(song any) string {
	var path string
	switch s := song.(type) {
	case radio.Song:
		path = s.FilePath
	case *radio.Song:
		if s != nil {
			path = s.FilePath
		}
	case radio.PendingSong:
		path = s.FilePath
	case *radio.PendingSong:
		if s != nil {
			path = s.FilePath
		}
	default:
		return "??? MiB"
	}

	// make the path absolute
	path = util.AbsolutePath(sf.musicPath(), path)

	fi, err := os.Stat(path)
	if err != nil {
		return "??? MiB"
	}

	size := fi.Size()
	if size < 0 {
		return "??? MiB"
	}

	return humanize.IBytes(uint64(size))
}

func (sf *StatefulFuncs) FuncMap() template.FuncMap {
	return map[string]any{
		"Status":       sf.Status,
		"SongFileSize": sf.SongFileSize,
	}
}

func TemplateFuncs() template.FuncMap {
	return defaultFunctions
}

var defaultFunctions = map[string]any{
	"Version":                     func() string { return buildinfo.ShortRef },
	"printjson":                   PrintJSON,
	"safeHTML":                    SafeHTML,
	"safeHTMLAttr":                SafeHTMLAttr,
	"safeURL":                     SafeURL,
	"safeCSS":                     func(s string) template.CSS { return template.CSS(s) },
	"IsValidThread":               IsValidThread,
	"IsImageThread":               IsImageThread,
	"IsRobot":                     radio.IsRobot,
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
	"Sub":                         func(a, b any) int64 { return castInt64(a) - castInt64(b) },
	"CalculateSubmissionCooldown": radio.CalculateSubmissionCooldown,
	"AllUserPermissions":          radio.AllUserPermissions,
	"HasField":                    HasField,
	"SongPair":                    SongPair,
	"TimeAgo":                     TimeAgo(time.Now),
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

func SafeURL(v any) (template.URL, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.E(errors.InvalidArgument)
	}
	return template.URL(s), nil
}

// IsValidThread tells you if a thread is valid, that is not-empty
// or is the literal 'none'
func IsValidThread(v string) bool {
	if len(v) == 0 {
		return false
	}
	if strings.EqualFold(v, "none") {
		return false
	}
	return true
}

// IsImageThread tells you if the thread is an image thread
func IsImageThread(v string) bool {
	return strings.HasPrefix(v, "image:")
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
			return "in <1 minute"
		}
		if d < time.Minute*2 {
			return fmt.Sprintf("in %.0f minute", d.Minutes())
		}
		return fmt.Sprintf("in %.0f minutes", d.Minutes())
	} else { // past duration
		d = d.Abs()
		if d <= time.Minute {
			return "<1 minute ago"
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

// example usage: {{ TimeAgo .CreatedAt "%y foo, %d, %h hours, %mbar" }}
// -> "2 foo, 23, 11 hours, 12bar" if >0 years have passed, otherwise
// "23, 11 hours, 12bar" etc.
func TimeAgo(now func() time.Time) func(time.Time, string) string {
	units := []struct {
		placeholder string
		duration    time.Duration
	}{
		{"%y", 365 * 24 * time.Hour},
		{"%d", 24 * time.Hour},
		{"%h", time.Hour},
		{"%m", time.Minute},
		{"%s", time.Second},
	}

	re := regexp.MustCompile(`(%[ydhms])`)

	return func(t time.Time, format string) string {
		elapsed := now().Sub(t).Truncate(time.Second)

		components := make(map[string]int64)
		remaining := elapsed
		for _, unit := range units {
			components[unit.placeholder] = int64(remaining / unit.duration)
			remaining = remaining % unit.duration
		}

		segments := re.Split(format, -1)
		matches := re.FindAllString(format, -1)

		// "seconds only" special case
		if len(matches) == 1 && matches[0] == "%s" {
			return fmt.Sprintf("%d%s", components["%s"], segments[1])
		}

		var parts []string
		for i, match := range matches {
			if value := components[match]; value > 0 || i == len(matches)-1 {
				parts = append(parts, strings.TrimRight(fmt.Sprintf("%d%s", value, segments[i+1]), ", "))
			}
		}

		if len(parts) == 0 && len(matches) > 0 {
			return fmt.Sprintf("%d%s", components["%s"], segments[re.FindStringIndex(format)[1]:])
		}

		return strings.Join(parts, ", ")
	}
}

func MediaDuration(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", d/time.Minute, d%time.Minute/time.Second)
}

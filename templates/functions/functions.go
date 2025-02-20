package functions

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"io/fs"
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
	"github.com/rs/zerolog"
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
	// for SongFileSize
	musicPath func() string
	// for WithVersion
	withVersion *withVersion
	// for Status
	status util.StreamValuer[radio.Status]
}

func NewWithVersionFunc(ctx context.Context, prefix string, fsys fs.FS) *withVersion {
	return &withVersion{
		logger: zerolog.Ctx(ctx),
		prefix: prefix,
		fs:     fsys,
	}
}

type withVersion struct {
	logger *zerolog.Logger
	prefix string
	fs     fs.FS
	cache  util.Map[string, template.URL]
}

func (sf *StatefulFuncs) Status() radio.Status {
	return sf.status.Latest()
}

func (sf *StatefulFuncs) UpdateWithVersion(wv *withVersion) {
	sf.withVersion = wv
}

func (sf *StatefulFuncs) WithVersion(urlPath string) template.URL {
	if sf.withVersion == nil {
		return template.URL(urlPath)
	}
	return sf.withVersion.WithVersion(urlPath)
}

var withVersionTmplRaw = `{{.Path}}{{with .Version}}?v={{.}}{{end}}`

var withVersionTmpl = template.Must(template.New("withVersion").Parse(withVersionTmplRaw))

type withVersionInput struct {
	Path    string
	Version string
}

func (wv *withVersion) WithVersion(urlPath string) template.URL {
	if wv == nil {
		return template.URL(urlPath)
	}

	if res, ok := wv.cache.Load(urlPath); ok {
		return res
	}

	render := func(store bool, version string) template.URL {
		var buf bytes.Buffer
		withVersionTmpl.Execute(&buf, withVersionInput{
			Path:    urlPath,
			Version: version,
		})

		output := template.URL(buf.String())
		if store {
			wv.cache.Store(urlPath, output)
		}

		wv.logger.Info().Str("path", urlPath).Str("version", version).Msg("withVersion render")
		return output
	}

	path := strings.TrimPrefix(urlPath, wv.prefix)
	f, err := wv.fs.Open(path)
	if err != nil {
		// if we can't open the file, just render it without version
		wv.logger.Err(err).Str("path", path).Msg("failed to  open file in withVersion")
		return render(false, "")
	}
	defer f.Close()

	// hash the contents of the file
	h := fnv.New64()
	_, err = io.Copy(h, f)
	if err != nil {
		wv.logger.Err(err).Str("path", path).Msg("failed to copy file in withVersion")
		// couldn't read the files contents, render it without a version
		return render(false, "")
	}

	return render(true, hex.EncodeToString(h.Sum(nil)))
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
		"WithVersion":  sf.WithVersion,
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
	"Reverse":                     Reverse,
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

// Reverse reverses a slice input and returns an iter.Seq2
// with both index and element, this means you always need
// to use {{range $i, $v := Reverse <slice>}} to get both
// values out, using a single only gets you the index.
func Reverse(s any) any {
	if s == nil {
		return nil
	}

	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Slice {
		return s
	}

	// returns an iter.Seq2[int, reflect.Value]
	return func(yield func(int, reflect.Value) bool) {
		for o, i := 0, v.Len()-1; i >= 0; o, i = o+1, i+1 {
			if !yield(o, v.Index(i)) {
				return
			}
		}
	}
}

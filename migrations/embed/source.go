package migrations

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4/source"
)

type asset struct {
	Name    string
	Content string
	etag    string
}

var migrations = map[string]string{}

var Source = becky{}

var _ source.Driver = becky{}

func migration(a asset) struct{} {
	migrations[a.Name] = a.Content
	return struct{}{}
}

type becky struct {
	migrations *source.Migrations
}

func (b becky) Open(_ string) (source.Driver, error) {
	return b.init(migrations)
}

func fromFilename(name string) (*source.Migration, error) {
	// parse the filename, it should be of the format
	//	 {version}_{identifier}.[up|down].sql
	//
	// so first remove the sql extension
	bare := strings.TrimSuffix(name, ".sql")

	var direction source.Direction
	// now check if we have up or down
	switch filepath.Ext(bare) {
	case ".down":
		direction = source.Down
	case ".up":
		direction = source.Up
	default:
		return nil, fmt.Errorf("unable to parse file %v: not up or down", name)
	}

	bare = strings.TrimSuffix(bare, filepath.Ext(bare))
	// split the version and identifier
	parts := strings.SplitN(bare, "_", 2)
	// version is a 0-padded integer, so trim those
	version, err := strconv.Atoi(strings.TrimLeft(parts[0], "0"))
	if err != nil {
		return nil, err
	}
	identifier := parts[1]

	m := &source.Migration{
		Version:    uint(version),
		Identifier: identifier,
		Direction:  direction,
		Raw:        name,
	}

	return m, nil
}

func (b becky) init(migrations map[string]string) (source.Driver, error) {
	b.migrations = source.NewMigrations()

	for name := range migrations {
		m, err := fromFilename(name)
		if err != nil {
			return nil, err
		}

		if !b.migrations.Append(m) {
			return nil, fmt.Errorf("unable to parse file %v", name)
		}
	}
	return b, nil
}

func (b becky) Close() error {
	return nil
}

func (b becky) First() (version uint, err error) {
	v, ok := b.migrations.First()
	if !ok {
		return 0, &os.PathError{Op: "first", Path: "<embedded>", Err: os.ErrNotExist}
	}
	return v, nil
}

func (b becky) Prev(version uint) (prev uint, err error) {
	v, ok := b.migrations.Prev(version)
	if !ok {
		return 0, &os.PathError{Op: fmt.Sprintf("prev for version %v", version), Path: "<embedded>", Err: os.ErrNotExist}
	}
	return v, nil
}

func (b becky) Next(version uint) (next uint, err error) {
	v, ok := b.migrations.Next(version)
	if !ok {
		return 0, &os.PathError{Op: fmt.Sprintf("next for version %v", version), Path: "<embedded>", Err: os.ErrNotExist}
	}
	return v, nil
}

func (b becky) ReadUp(version uint) (io.ReadCloser, string, error) {
	m, ok := b.migrations.Up(version)
	if !ok {
		return nil, "", &os.PathError{Op: fmt.Sprintf("read version %v", version), Path: "<embedded>", Err: os.ErrNotExist}
	}

	r := strings.NewReader(migrations[m.Raw])
	return ioutil.NopCloser(r), m.Identifier, nil
}

func (b becky) ReadDown(version uint) (io.ReadCloser, string, error) {
	m, ok := b.migrations.Down(version)
	if !ok {
		return nil, "", &os.PathError{Op: fmt.Sprintf("read version %v", version), Path: "<embedded>", Err: os.ErrNotExist}
	}

	r := strings.NewReader(migrations[m.Raw])
	return ioutil.NopCloser(r), m.Identifier, nil
}

// ListMigrations returns all migrations currently available
func ListMigrations() ([]Migration, error) {
	var tmp = map[int]*Migration{}
	var versions = []int{}

	for name := range migrations {
		m, err := fromFilename(name)
		if err != nil {
			return nil, err
		}

		migr := &Migration{
			Version:    int(m.Version),
			Identifier: m.Identifier,
			Filename:   m.Raw,
		}

		// swap to the existing migration if we have it, this is done such that the
		// UP and DOWN versions of a migration share output
		if mt, ok := tmp[migr.Version]; ok {
			migr = mt
		}

		switch m.Direction {
		case source.Up:
			migr.Up = true
		case source.Down:
			migr.Down = true
		}

		tmp[migr.Version] = migr
		versions = append(versions, migr.Version)
	}

	sort.Ints(versions)

	var all = make([]Migration, len(versions))
	for i, version := range versions {
		all[i] = *tmp[version]
	}

	return all, nil
}

type Migration struct {
	Up         bool
	Down       bool
	Version    int
	Identifier string
	Filename   string
}

func (m Migration) Pretty() string {
	updown := "????"
	if m.Up && m.Down {
		updown = "UP & DOWN"
	} else if m.Up {
		updown = "UP"
	} else if m.Down {
		updown = "DOWN"
	}

	return fmt.Sprintf("%.4d %s \"%s\" (%s)",
		m.Version,
		updown,
		m.Identifier,
		m.Filename,
	)
}

// GetIdentifier tries to get the identifier for the version given, if it exists
func GetIdentifier(s source.Driver, version int) (string, bool) {
	b, ok := s.(becky)
	if !ok {
		return "", false
	}

	// try the up version
	m, ok := b.migrations.Up(uint(version))
	if ok {
		return m.Identifier, true
	}

	// if that fails, try the down version
	m, ok = b.migrations.Down(uint(version))
	if ok {
		return m.Identifier, true
	}

	return "", false
}

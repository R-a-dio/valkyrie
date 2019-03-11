package migrations

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage/mariadb"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/jmoiron/sqlx"
)

type migrationState int

const (
	// Done indicates a migration was done
	Done migrationState = iota
	// Dirty indicates a migration is in progress
	Dirty
)

func (ms migrationState) String() string {
	switch ms {
	case Done:
		return "done"
	case Dirty:
		return "dirty"
	default:
		panic("unknown migration state")
	}
}

func (ms *migrationState) Scan(src interface{}) error {
	if src == nil {
		return nil
	}

	s, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte but got %T", src)
	}

	switch string(s) {
	case "done":
		*ms = Done
	case "dirty":
		*ms = Dirty
	default:
		return fmt.Errorf("unexpected migration state: %s", string(s))
	}
	return nil
}

func (ms migrationState) Value() (driver.Value, error) {
	return ms.String(), nil
}

type Version struct {
	State      migrationState
	Version    int
	Direction  source.Direction
	Identifier string
	Time       time.Time
}

func (v Version) Pretty() string {
	return fmt.Sprintf("[%s] migration %s (%s) for \"%.4d - %s\"",
		v.Time.Format("2006-01-02 15:04:05"),
		strings.ToUpper(v.State.String()),
		strings.ToUpper(string(v.Direction)),
		v.Version,
		v.Identifier,
	)
}

type mariaDB struct {
	ctx context.Context
	db  *sqlx.DB

	// source driver so we can get an identifier for a version
	source source.Driver
	// migration direction, this should be set by our migration caller
	Direction source.Direction
	// name of the database we're working on
	databaseName string

	// mu protects the field below it
	mu sync.Mutex
	// isLocked tells us if we are holding a mariadb lock
	isLocked bool
}

// NewDatabase returns an instance of a database.Driver using the context, configuration
// and source driver given
func NewDatabase(ctx context.Context, cfg config.Config, s source.Driver) (database.Driver, error) {
	db, err := mariadb.ConnectDB(ctx, cfg, true)
	if err != nil {
		return nil, err
	}
	// limit us to one connection
	db.SetMaxOpenConns(1)

	m := &mariaDB{
		ctx:       ctx,
		db:        db,
		source:    s,
		Direction: source.Up,
	}
	return m, m.ensureMigrationsTable()
}

func (m *mariaDB) Open(string) (database.Driver, error) {
	return m, nil
}

func (m *mariaDB) ensureMigrationsTable() (err error) {
	if err = m.Lock(); err != nil {
		return err
	}

	defer func() {
		if e := m.Unlock(); e != nil {
			err = e
		}
	}()

	var result string
	query := `SHOW TABLES LIKE 'schema_migrations';`
	err = sqlx.GetContext(m.ctx, m.db, &result, query)
	if err != sql.ErrNoRows {
		return err
	}

	// create table if it doesn't exist
	query = `
	CREATE TABLE schema_migrations (
		id INT UNSIGNED NOT NULL AUTO_INCREMENT,
		state ENUM('dirty', 'done') NOT NULL DEFAULT 'dirty',
		version BIGINT NOT NULL,
		direction ENUM('up', 'down') NOT NULL,
		identifier TEXT NOT NULL,
		time TIMESTAMP NOT NULL,
		PRIMARY KEY (id)
	);
	`
	_, err = m.db.ExecContext(m.ctx, query)
	if err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	return nil
}

func (m *mariaDB) Run(r io.Reader) error {
	statement, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	_, err = m.db.ExecContext(m.ctx, string(statement))
	if err != nil {
		return database.Error{OrigErr: err, Err: "migration failed", Query: statement}
	}
	return nil
}

func (m *mariaDB) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *mariaDB) Lock() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isLocked {
		return database.ErrLocked
	}

	name := fmt.Sprintf("%s:%s", m.databaseName, "schema_migrations")
	aid, err := database.GenerateAdvisoryLockId(name)
	if err != nil {
		return err
	}

	query := "SELECT GET_LOCK(?, 10)"
	var success bool
	err = sqlx.GetContext(m.ctx, m.db, &success, query, aid)
	if err != nil {
		return &database.Error{OrigErr: err, Err: "try lock failed", Query: []byte(query)}
	}

	if !success {
		return database.ErrLocked
	}

	m.isLocked = true
	return nil
}

func (m *mariaDB) Unlock() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLocked {
		return nil
	}

	name := fmt.Sprintf("%s:%s", m.databaseName, "schema_migrations")
	aid, err := database.GenerateAdvisoryLockId(name)
	if err != nil {
		return err
	}

	const query = "SELECT RELEASE_LOCK(?)"
	var success *bool
	err = sqlx.GetContext(m.ctx, m.db, &success, query, aid)
	if err != nil {
		return &database.Error{OrigErr: err, Err: "try release failed", Query: []byte(query)}
	}

	// change this when we handle the below
	//
	// we don't really check the result of calling release, so always set ourselves
	// into unlocked state
	m.isLocked = false

	// check if we succeeded
	if success == nil || !*success {
		// we were not locked?
		return nil
	}

	return nil
}

func (m *mariaDB) Drop() error {
	// not implemented
	return nil
}

func (m *mariaDB) Version() (version int, dirty bool, err error) {
	v, err := m.VersionExt()
	if err != nil {
		return database.NilVersion, false, err
	}

	return v.Version, v.State == Dirty, nil
}

func (m *mariaDB) VersionExt() (Version, error) {
	const query = `
	SELECT 
		state, version, direction, identifier, time
	FROM
		schema_migrations
	ORDER BY
		id DESC
	LIMIT 1;
	`

	var v Version
	err := sqlx.GetContext(m.ctx, m.db, &v, query)
	if err == sql.ErrNoRows {
		return Version{Version: database.NilVersion}, nil
	}
	if err != nil {
		return Version{}, &database.Error{OrigErr: err, Query: []byte(query)}
	}

	return v, nil
}

func (m *mariaDB) VersionLog() ([]Version, error) {
	const query = `
	SELECT
		state, version, direction, identifier, time
	FROM
		schema_migrations
	ORDER BY
		id DESC;
	`

	var v []Version
	err := sqlx.SelectContext(m.ctx, m.db, &v, query)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (m *mariaDB) SetVersion(version int, dirty bool) error {
	var v Version

	v.Version = version
	v.Identifier, _ = GetIdentifier(m.source, version)
	if dirty {
		v.State = Dirty
	} else {
		v.State = Done
	}

	old, _, err := m.Version()
	if err != nil {
		return err
	}
	if old < version {
		v.Direction = source.Up
	} else {
		v.Direction = source.Down
	}

	return m.SetVersionExt(v)
}

func (m *mariaDB) SetVersionExt(version Version) error {
	var query = `
	INSERT INTO
		schema_migrations
	(
		state, version, direction, identifier
	) VALUES (
		:state, :version, :direction, :identifier	
	)`

	query, args, err := sqlx.Named(query, version)
	if err != nil {
		return err
	}

	_, err = m.db.ExecContext(m.ctx, query, args...)
	return err
}

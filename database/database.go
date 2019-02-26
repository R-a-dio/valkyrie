package database

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/cenkalti/backoff"
	_ "github.com/go-sql-driver/mysql" // mariadb
	"github.com/jmoiron/sqlx"
)

// specialCasedColumnNames is a map of Go <StructField> to SQL <ColumnName>
var specialCasedColumnNames = map[string]string{
	"CreatedAt":     "created_at",
	"DeletedAt":     "deleted_at",
	"UpdatedAt":     "updated_at",
	"RememberToken": "remember_token",
}

// Connect connects to the database configured in cfg
func Connect(cfg config.Config) (*sqlx.DB, error) {
	info := cfg.Conf().Database

	db, err := sqlx.Connect(info.DriverName, info.DSN)
	if err != nil {
		return nil, err
	}

	db.MapperFunc(func(s string) string {
		n, ok := specialCasedColumnNames[s]
		if ok {
			s = n
		}
		return strings.ToLower(s)
	})
	return db, nil
}

// Handler is the interface passed to database accessing functions and should
// only be created by a call to Handle
type Handler interface {
	sqlx.Execer
	sqlx.Queryer

	// Retry allows retrying a database operation with automated backing off if
	// multiple retries are needed. The exact values are defined in the config
	// package
	Retry(n func(error, time.Duration), fn func(Handler) error) error
}

// HandlerTx is a database handler that is used for transactions
type HandlerTx interface {
	Handler

	// Commit commits the transaction
	Commit() error
	// Rollback aborts the transaction
	Rollback() error
	// Tx returns the underlying transaction
	Tx() *sqlx.Tx
}

type extContext interface {
	sqlx.ExecerContext
	sqlx.QueryerContext
}

// Handle creates a handler with the ctx and db given
func Handle(ctx context.Context, db *sqlx.DB) Handler {
	return handle{
		ext: db,
		db:  db,
		ctx: ctx,
	}
}

// HandleTx creates a HandlerTx from the ctx and db given
func HandleTx(ctx context.Context, db *sqlx.DB) (HandlerTx, error) {
	return BeginTx(Handle(ctx, db))
}

// BeginTx begins a transaction on the handler given
func BeginTx(h Handler) (HandlerTx, error) {
	const op errors.Op = "database/BeginTx"

	var err error
	var hh handle

	// dig out the handle value of the Handler we got
	switch a := h.(type) {
	case handle:
		hh = a
	case handleTx:
		hh = a.handle
	default:
		panic("unknown Handler implementation passed to BeginTx")
	}

	var htx handleTx
	htx.handle = hh
	htx.tx, err = htx.db.BeginTxx(htx.ctx, nil)
	// set ext on our handle so that it now uses the transaction
	htx.ext = htx.tx
	if err != nil {
		return nil, errors.E(op, errors.TransactionBegin, err)
	}
	return htx, nil
}

// handleTx is a handle that implements transactions on top of handle
type handleTx struct {
	handle
	tx *sqlx.Tx
}

func (h handleTx) Commit() error {
	const op errors.Op = "database/handleTx.Commit"

	if h.tx == nil {
		panic("sanity: Commit called with nil tx")
	}

	err := h.tx.Commit()
	if err != nil {
		return errors.E(op, errors.TransactionCommit, err)
	}
	return nil
}

func (h handleTx) Rollback() error {
	const op errors.Op = "database/handleTx.Rollback"

	if h.tx == nil {
		panic("sanity: Rollback called with nil tx")
	}

	err := h.tx.Rollback()
	if err != nil {
		return errors.E(op, errors.TransactionRollback, err)
	}
	return nil
}

func (h handleTx) Tx() *sqlx.Tx {
	return h.tx
}

var _ HandlerTx = handleTx{}

type handle struct {
	// ext is the field used when actually performing queries
	ext extContext
	db  *sqlx.DB
	ctx context.Context
}

var _ Handler = handle{}

func (h handle) Exec(query string, args ...interface{}) (sql.Result, error) {
	return h.ext.ExecContext(h.ctx, query, args...)
}

func (h handle) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return h.ext.QueryContext(h.ctx, query, args...)
}

func (h handle) Queryx(query string, args ...interface{}) (*sqlx.Rows, error) {
	return h.ext.QueryxContext(h.ctx, query, args...)
}

func (h handle) QueryRowx(query string, args ...interface{}) *sqlx.Row {
	return h.ext.QueryRowxContext(h.ctx, query, args...)
}

func (h handle) Retry(n func(error, time.Duration), fn func(Handler) error) error {
	return backoff.RetryNotify(func() error {
		return fn(h)
	}, config.NewDatabaseBackoff(), n)
}

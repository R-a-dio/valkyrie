package database

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// Handler is the interface passed to database accessing functions and should
// only be created by a call to Handle
type Handler interface {
	internal()
	ext
	Commit() error
	Rollback() error
}

type ext interface {
	sqlx.Execer
	sqlx.Queryer
}

type extContext interface {
	sqlx.ExecerContext
	sqlx.QueryerContext
}

// Handle creates a handler with the ctx and db given
func Handle(ctx context.Context, db *sqlx.DB) Handler {
	return handle{
		db:  db,
		tx:  nil,
		ctx: ctx,
	}
}

// BeginTx begins a transaction on the handler given
func BeginTx(hh Handler) (Handler, error) {
	var err error
	// TODO: find a way to have this be testable aswell?
	h := hh.(handle)
	h.tx, err = h.db.BeginTxx(h.ctx, nil)
	return h, err
}

type handle struct {
	db  *sqlx.DB
	tx  *sqlx.Tx
	ctx context.Context
}

var _ Handler = handle{}

func (h handle) getTx() extContext {
	if h.db == nil {
		panic("sanity: getTx called with nil db")
	}
	if h.tx == nil {
		return h.db
	}
	return h.tx
}

func (h handle) Exec(query string, args ...interface{}) (sql.Result, error) {
	return h.getTx().ExecContext(h.ctx, query, args...)
}

func (h handle) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return h.getTx().QueryContext(h.ctx, query, args...)
}

func (h handle) Queryx(query string, args ...interface{}) (*sqlx.Rows, error) {
	return h.getTx().QueryxContext(h.ctx, query, args...)
}

func (h handle) QueryRowx(query string, args ...interface{}) *sqlx.Row {
	return h.getTx().QueryRowxContext(h.ctx, query, args...)
}

func (h handle) Commit() error {
	if h.tx == nil {
		panic("sanity: Commit called with nil tx")
	}
	return h.tx.Commit()
}

func (h handle) Rollback() error {
	if h.tx == nil {
		panic("sanity: Rollback called with nil tx")
	}
	return h.tx.Rollback()
}

func (h handle) internal() {}

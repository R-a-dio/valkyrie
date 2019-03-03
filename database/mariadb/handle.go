package mariadb

import (
	"context"
	"database/sql"
	"strings"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	_ "github.com/go-sql-driver/mysql" // mariadb
	"github.com/jmoiron/sqlx"
)

func init() {
	database.Register("mariadb", Connect)
}

// specialCasedColumnNames is a map of Go <StructField> to SQL <ColumnName>
var specialCasedColumnNames = map[string]string{
	"CreatedAt":     "created_at",
	"DeletedAt":     "deleted_at",
	"UpdatedAt":     "updated_at",
	"RememberToken": "remember_token",
}

// Connect connects to the database configured in cfg
func Connect(cfg config.Config) (radio.StorageService, error) {
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

	return &StorageService{db}, nil
}

type StorageService struct {
	db *sqlx.DB
}

func (s *StorageService) tx(ctx context.Context, tx radio.StorageTx) (*sqlx.Tx, error) {
	if tx == nil {
		// new transaction
		return s.db.BeginTxx(ctx, nil)
	}

	// existing transaction, make sure it's one of ours and then use it
	txx, ok := tx.(*sqlx.Tx)
	if !ok {
		panic("mariadb: invalid tx passed to StorageService")
	}

	return txx, nil
}

func (s *StorageService) Queue(ctx context.Context) radio.QueueStorage {
	return QueueStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) QueueTx(ctx context.Context, tx radio.StorageTx) (radio.QueueStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := QueueStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

func (s *StorageService) Song(ctx context.Context) radio.SongStorage {
	return SongStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) SongTx(ctx context.Context, tx radio.StorageTx) (radio.SongStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := SongStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

func (s *StorageService) Track(ctx context.Context) radio.TrackStorage {
	return TrackStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) TrackTx(ctx context.Context, tx radio.StorageTx) (radio.TrackStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := TrackStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

func (s *StorageService) Request(ctx context.Context) radio.RequestStorage {
	return RequestStorage{
		handle: handle{s.db, ctx},
	}
}
func (s *StorageService) RequestTx(ctx context.Context, tx radio.StorageTx) (radio.RequestStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := RequestStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

func (s *StorageService) User(ctx context.Context) radio.UserStorage {
	return UserStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) UserTx(ctx context.Context, tx radio.StorageTx) (radio.UserStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := UserStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

type extContext interface {
	sqlx.ExecerContext
	sqlx.QueryerContext
}

// requireTx returns a handle that uses a transaction, if the handle given already is
// one using a transaction it returns it as-is, otherwise makes a new transaction
func requireTx(h handle) (handle, radio.StorageTx, error) {
	if tx, ok := h.ext.(*sqlx.Tx); ok {
		return h, tx, nil
	}

	db, ok := h.ext.(*sqlx.DB)
	if !ok {
		// TODO: add type
		panic("mariadb: unknown type in ext field")
	}

	tx, err := db.BeginTxx(h.ctx, nil)
	if err != nil {
		return h, nil, err
	}
	h.ext = tx
	return h, tx, nil
}

// handle is an implementation of sqlx.Execer and sqlx.Queryer that can either use
// a *sqlx.DB directly, or a *sqlx.Tx. It implements these with the *Context equivalents
type handle struct {
	ext extContext
	ctx context.Context
}

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

var _ sqlx.Execer = handle{}
var _ sqlx.Queryer = handle{}

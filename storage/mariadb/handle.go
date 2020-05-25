package mariadb

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/go-sql-driver/mysql" // mariadb
	"github.com/jmoiron/sqlx"
)

func init() {
	storage.Register("mariadb", Connect)
}

// specialCasedColumnNames is a map of Go <StructField> to SQL <ColumnName>
var specialCasedColumnNames = map[string]string{
	"CreatedAt":     "created_at",
	"DeletedAt":     "deleted_at",
	"UpdatedAt":     "updated_at",
	"RememberToken": "remember_token",
}

// mapperFunc implements the MapperFunc for sqlx to specialcase column names
// and lowercase them for scan matching
func mapperFunc(s string) string {
	n, ok := specialCasedColumnNames[s]
	if ok {
		s = n
	}
	return strings.ToLower(s)
}

// ConnectDB connects to the configured mariadb instance and returns the raw database
// object. Argument multistatement indicates if we should allow queries with multiple
// statements in them.
func ConnectDB(ctx context.Context, cfg config.Config, multistatement bool) (*sqlx.DB, error) {
	info := cfg.Conf().Database

	// we require some specific arguments in the DSN to have code work properly, so make
	// sure those are included
	dsn, err := mysql.ParseDSN(info.DSN)
	if err != nil {
		return nil, err
	}

	// enable multistatement queries if asked for
	if multistatement {
		dsn.MultiStatements = true
	}
	// UTC location to handle time.Time location
	dsn.Loc, err = time.LoadLocation("UTC")
	if err != nil {
		return nil, err
	}
	// parsetime to handle time.Time in the driver
	dsn.ParseTime = true
	// time_zone to have the database not try and interpret dates and times as the
	// locale of the system, but as UTC+0 instead
	if dsn.Params == nil {
		dsn.Params = map[string]string{}
	}
	dsn.Params["time_zone"] = "'+00:00'"
	conndsn := dsn.FormatDSN()

	// we want to print what we're connecting to, but not print our password
	if dsn.Passwd != "" {
		dsn.Passwd = "<redacted>"
	}
	log.Printf("storage: mariadb: trying to connect to %s", dsn.FormatDSN())

	db, err := sqlx.ConnectContext(ctx, "mysql", conndsn)
	if err != nil {
		return nil, err
	}

	db.MapperFunc(mapperFunc)

	return db, nil
}

// Connect connects to the database configured in cfg
func Connect(cfg config.Config) (radio.StorageService, error) {
	// TODO(wessie): pass a context here somehow?
	db, err := ConnectDB(context.Background(), cfg, false)
	if err != nil {
		return nil, err
	}
	return &StorageService{db}, nil
}

// StorageService implements radio.StorageService with a sql database
type StorageService struct {
	db *sqlx.DB
}

// tx either unwraps the tx given to a *sqlx.Tx, or creates a new transaction if tx is
// nil. Passing in a StorageTx not returned by this package will panic
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

func (s *StorageService) Sessions(ctx context.Context) radio.SessionStorage {
	return SessionStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) SessionsTx(ctx context.Context, tx radio.StorageTx) (radio.SessionStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := SessionStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
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

func (s *StorageService) Submissions(ctx context.Context) radio.SubmissionStorage {
	return SubmissionStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) SubmissionsTx(ctx context.Context, tx radio.StorageTx) (radio.SubmissionStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := SubmissionStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

func (s *StorageService) News(ctx context.Context) radio.NewsStorage {
	return NewsStorage{
		handle: handle{s.db, ctx},
	}
}

func (s *StorageService) NewsTx(ctx context.Context, tx radio.StorageTx) (radio.NewsStorage, radio.StorageTx, error) {
	txx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := NewsStorage{
		handle: handle{txx, ctx},
	}
	return storage, txx, nil
}

func (s *StorageService) Status(ctx context.Context) radio.StatusStorage {
	return StatusStorage{
		handle: handle{s.db, ctx},
	}
}

type extContext interface {
	sqlx.ExecerContext
	sqlx.QueryerContext
	// these are methods on sqlx.binder that is private, we need to implement these
	// to be a sqlx.Ext so that we can use all extensions added by sqlx
	DriverName() string
	Rebind(string) string
	BindNamed(string, interface{}) (string, []interface{}, error)
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

func (h handle) BindNamed(query string, arg interface{}) (string, []interface{}, error) {
	return h.ext.BindNamed(query, arg)
}

func (h handle) Rebind(query string) string {
	return h.ext.Rebind(query)
}

func (h handle) DriverName() string {
	return h.ext.DriverName()
}

var _ sqlx.Execer = handle{}
var _ sqlx.Queryer = handle{}
var _ sqlx.Ext = handle{}

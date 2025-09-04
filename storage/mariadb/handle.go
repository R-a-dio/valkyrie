package mariadb

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/go-sql-driver/mysql" // mariadb
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var DatabaseConnectFunc = sqlx.ConnectContext

// specialCasedColumnNames is a map of Go <StructField> to SQL <ColumnName>
var specialCasedColumnNames = map[string]string{
	"CreatedAt":     "created_at",
	"DeletedAt":     "deleted_at",
	"UpdatedAt":     "updated_at",
	"RememberToken": "remember_token",
}

var invalidQueries = map[string]string{}

type NoParams struct{}

// zeroValue returns the zero value of T with things allocated inside
// if possible, small wrapper around zeroValueImpl
func zeroValue[T any]() T {
	v := zeroValueImpl(reflect.TypeFor[T]())
	return v.Interface().(T)
}

// zeroValueImpl returns a "filled" in zero value of the type given
//
// It does this by recursively allocating stuff until it reaches a
// concrete type
func zeroValueImpl(t reflect.Type) reflect.Value {
	v := reflect.New(t).Elem()

	switch v.Kind() {
	case reflect.Ptr:
		// if we have a pointer, allocate the thing it's pointing to
		v.Set(zeroValueImpl(v.Type().Elem()).Addr())
	case reflect.Slice:
		// if we have a slice, we allocate a single element to make sqlx happy
		v = reflect.Append(v, zeroValueImpl(v.Type().Elem()))
	case reflect.Struct:
		// if we have a struct, we allocate all exported fields recursively
		for i := range v.Type().NumField() {
			// only exported fields, or reflect will yell at us
			if v.Type().Field(i).IsExported() {
				fv := v.Field(i)
				fv.Set(zeroValueImpl(fv.Type()))
			}
		}
	}

	return v
}

func CheckQuery[T any](query string) struct{} {
	_, _, err := sqlx.Named(query, zeroValue[T]())
	if err != nil {
		_, filename, line, _ := runtime.Caller(1)
		if tmp := strings.Split(filename, string(filepath.Separator)); len(tmp) > 2 {
			filename = filepath.Join(tmp[len(tmp)-2:]...)
		}

		identifier := fmt.Sprintf("%s:%d", filename, line)
		invalidQueries[identifier] = err.Error()
	}

	return struct{}{}
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

	zerolog.Ctx(ctx).Info().Ctx(ctx).Str("address", dsn.FormatDSN()).Msg("trying to connect")

	db, err := DatabaseConnectFunc(ctx, "mysql", conndsn)
	if err != nil {
		return nil, err
	}

	db.MapperFunc(mapperFunc)

	return db, nil
}

// Connect connects to the database configured in cfg
func Connect(ctx context.Context, cfg config.Config) (radio.StorageService, error) {
	db, err := ConnectDB(ctx, cfg, false)
	if err != nil {
		return nil, err
	}
	return &StorageService{db: db}, nil
}

// StorageService implements radio.StorageService with a sql database
type StorageService struct {
	db *sqlx.DB
}

func (s *StorageService) Close() error {
	return s.db.Close()
}

// fakeTx is a *sqlx.Tx with the Commit method disabled
type fakeTx struct {
	*sqlx.Tx
	called atomic.Bool
}

// Commit does nothing
func (tx *fakeTx) Commit() error {
	success := tx.called.CompareAndSwap(false, true)
	if !success {
		return sql.ErrTxDone
	}
	return nil
}

// Rollback only calls the real Rollback if Commit has not been called yet,
// this is to support the common `defer tx.Rollback()` pattern
func (tx *fakeTx) Rollback() error {
	success := tx.called.CompareAndSwap(false, true)
	if !success {
		return sql.ErrTxDone
	}
	return tx.Tx.Rollback()
}

type spanTx struct {
	*sqlx.Tx
	span trace.Span
	end  func()
}

func (tx spanTx) Commit() error {
	defer tx.end()
	tx.span.AddEvent("commit")

	return tx.Tx.Commit()
}

func (tx spanTx) Rollback() error {
	defer tx.end()
	tx.span.AddEvent("rollback")

	return tx.Tx.Rollback()
}

// tx either unwraps the tx given to a *sqlx.Tx, or creates a new transaction if tx is
// nil. Passing in a StorageTx not returned by this package will panic
func (s *StorageService) tx(ctx context.Context, tx radio.StorageTx) (context.Context, *sqlx.Tx, radio.StorageTx, error) {
	return beginTx(ctx, s.db, tx)
}

// beginTx starts a new transaction but only if a transaction doesn't already exist in any of the
// arguments given.
func beginTx(ctx context.Context, ex extContext, tx radio.StorageTx) (context.Context, *sqlx.Tx, radio.StorageTx, error) {
	if tx != nil {
		// existing transaction, make sure it's one of ours and then use it
		switch txx := tx.(type) {
		case *sqlx.Tx:
			// if this is a real tx, we disable the commit so that the transaction can't
			// be committed earlier than expected by the creator
			return ctx, txx, &fakeTx{Tx: txx}, nil
		case spanTx:
			return ctx, txx.Tx, &fakeTx{Tx: txx.Tx}, nil
		case *fakeTx:
			return ctx, txx.Tx, txx, nil
		default:
			panic("mariadb: invalid tx passed to beginTx")
		}
	}

	// now check if our ex is already a transaction
	switch sx := ex.(type) {
	case *sqlx.Tx:
		// ex was already a transaction, return it wrapped in a fake
		return ctx, sx, &fakeTx{Tx: sx}, nil
	case *sqlx.DB:
		// it's just our normal db instance, create a new transaction
		// new transaction
		tx, err := sx.BeginTxx(ctx, nil)
		if err != nil {
			return ctx, nil, nil, err
		}
		ctx, span := otel.Tracer("mariadb").Start(ctx, "transaction")
		end := sync.OnceFunc(func() { span.End() })
		return ctx, tx, spanTx{tx, span, end}, err
	}

	panic("mariadb: invalid ex passed to beginTx")
}

func newHandle(ctx context.Context, ext extContext, name string) handle {
	return handle{
		ext:     ext,
		ctx:     ctx,
		service: name,
	}
}

func (s *StorageService) Sessions(ctx context.Context) radio.SessionStorage {
	return SessionStorage{
		handle: newHandle(ctx, s.db, "sessions"),
	}
}

func (s *StorageService) SessionsTx(ctx context.Context, tx radio.StorageTx) (radio.SessionStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := SessionStorage{
		handle: newHandle(ctx, db, "sessions"),
	}
	return storage, tx, nil
}

func (s *StorageService) Queue(ctx context.Context) radio.QueueStorage {
	return QueueStorage{
		handle: newHandle(ctx, s.db, "queue"),
	}
}

func (s *StorageService) QueueTx(ctx context.Context, tx radio.StorageTx) (radio.QueueStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := QueueStorage{
		handle: newHandle(ctx, db, "queue"),
	}
	return storage, tx, nil
}

func (s *StorageService) Song(ctx context.Context) radio.SongStorage {
	return SongStorage{
		handle: newHandle(ctx, s.db, "song"),
	}
}

func (s *StorageService) SongTx(ctx context.Context, tx radio.StorageTx) (radio.SongStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := SongStorage{
		handle: newHandle(ctx, db, "song"),
	}
	return storage, tx, nil
}

func (s *StorageService) Track(ctx context.Context) radio.TrackStorage {
	return TrackStorage{
		handle: newHandle(ctx, s.db, "track"),
	}
}

func (s *StorageService) TrackTx(ctx context.Context, tx radio.StorageTx) (radio.TrackStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := TrackStorage{
		handle: newHandle(ctx, db, "track"),
	}
	return storage, tx, nil
}

func (s *StorageService) Request(ctx context.Context) radio.RequestStorage {
	return RequestStorage{
		handle: newHandle(ctx, s.db, "request"),
	}
}
func (s *StorageService) RequestTx(ctx context.Context, tx radio.StorageTx) (radio.RequestStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := RequestStorage{
		handle: newHandle(ctx, db, "request"),
	}
	return storage, tx, nil
}

func (s *StorageService) User(ctx context.Context) radio.UserStorage {
	return UserStorage{
		handle: newHandle(ctx, s.db, "user"),
	}
}

func (s *StorageService) UserTx(ctx context.Context, tx radio.StorageTx) (radio.UserStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := UserStorage{
		handle: newHandle(ctx, db, "user"),
	}
	return storage, tx, nil
}

func (s *StorageService) Submissions(ctx context.Context) radio.SubmissionStorage {
	return SubmissionStorage{
		handle: newHandle(ctx, s.db, "submissions"),
	}
}

func (s *StorageService) SubmissionsTx(ctx context.Context, tx radio.StorageTx) (radio.SubmissionStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := SubmissionStorage{
		handle: newHandle(ctx, db, "submissions"),
	}
	return storage, tx, nil
}

func (s *StorageService) News(ctx context.Context) radio.NewsStorage {
	return NewsStorage{
		handle: newHandle(ctx, s.db, "news"),
	}
}

func (s *StorageService) NewsTx(ctx context.Context, tx radio.StorageTx) (radio.NewsStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := NewsStorage{
		handle: newHandle(ctx, db, "news"),
	}
	return storage, tx, nil
}

func (s *StorageService) Status(ctx context.Context) radio.StatusStorage {
	return StatusStorage{
		handle: newHandle(ctx, s.db, "status"),
	}
}

func (s *StorageService) Relay(ctx context.Context) radio.RelayStorage {
	return RelayStorage{
		handle: newHandle(ctx, s.db, "relay"),
	}
}

func (s *StorageService) RelayTx(ctx context.Context, tx radio.StorageTx) (radio.RelayStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := RelayStorage{
		handle: newHandle(ctx, db, "relay"),
	}
	return storage, tx, nil
}

func (s *StorageService) Search() radio.SearchService {
	return SearchService{
		db: s.db,
	}
}

func (s *StorageService) Schedule(ctx context.Context) radio.ScheduleStorage {
	return ScheduleStorage{
		handle: newHandle(ctx, s.db, "schedule"),
	}
}

func (s *StorageService) ScheduleTx(ctx context.Context, tx radio.StorageTx) (radio.ScheduleStorage, radio.StorageTx, error) {
	ctx, db, tx, err := s.tx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	storage := ScheduleStorage{
		handle: newHandle(ctx, db, "schedule"),
	}
	return storage, tx, nil
}

type extContext interface {
	sqlx.ExecerContext
	sqlx.QueryerContext
	// these are methods on sqlx.binder that is private, we need to implement these
	// to be a sqlx.Ext so that we can use all extensions added by sqlx
	DriverName() string
	Rebind(string) string
	BindNamed(string, any) (string, []any, error)
}

// requireTx returns a handle that uses a transaction, if the handle given already is
// one using a transaction it returns it as-is, otherwise makes a new transaction
func requireTx(h handle) (handle, radio.StorageTx, error) {
	ctx, tx, stx, err := beginTx(h.ctx, h.ext, nil)
	if err != nil {
		return h, nil, err
	}

	// update our handle
	h.ctx = ctx
	h.ext = tx
	return h, stx, nil
}

func namedExecLastInsertId(e sqlx.Ext, query string, arg any) (int64, error) {
	res, err := sqlx.NamedExec(e, query, arg)
	if err != nil {
		return 0, err
	}

	new, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return new, nil
}

// handle is an implementation of sqlx.Execer and sqlx.Queryer that can either use
// a *sqlx.DB directly, or a *sqlx.Tx. It implements these with the *Context equivalents
type handle struct {
	ext extContext
	ctx context.Context

	service string
}

func (h handle) span(op errors.Op) (handle, func(...trace.SpanEndOption)) {
	var span trace.Span
	h.ctx, span = otel.Tracer("mariadb").Start(h.ctx, string(op))

	return h, span.End
}

func (h handle) Exec(query string, args ...any) (sql.Result, error) {
	defer func(start time.Time) {
		zerolog.Ctx(h.ctx).Debug().
			Str("storage_service", h.service).
			Str("query", query).
			Any("arguments", args).
			TimeDiff("execution_time", time.Now(), start).
			Msg("exec")
	}(time.Now())

	return h.ext.ExecContext(h.ctx, query, args...)
}

func (h handle) Query(query string, args ...any) (*sql.Rows, error) {
	defer func(start time.Time) {
		zerolog.Ctx(h.ctx).Debug().
			Str("storage_service", h.service).
			Str("query", query).
			Any("arguments", args).
			TimeDiff("execution_time", time.Now(), start).
			Msg("query")
	}(time.Now())

	return h.ext.QueryContext(h.ctx, query, args...)
}

func (h handle) Queryx(query string, args ...any) (*sqlx.Rows, error) {
	defer func(start time.Time) {
		zerolog.Ctx(h.ctx).Debug().
			Str("storage_service", h.service).
			Str("query", query).
			Any("arguments", args).
			TimeDiff("execution_time", time.Now(), start).
			Msg("queryx")
	}(time.Now())

	return h.ext.QueryxContext(h.ctx, query, args...)
}

func (h handle) QueryRowx(query string, args ...any) *sqlx.Row {
	defer func(start time.Time) {
		zerolog.Ctx(h.ctx).Debug().
			Str("storage_service", h.service).
			Str("query", query).
			Any("arguments", args).
			TimeDiff("execution_time", time.Now(), start).
			Msg("query_rowx")
	}(time.Now())

	return h.ext.QueryRowxContext(h.ctx, query, args...)
}

func (h handle) BindNamed(query string, arg any) (string, []any, error) {
	defer func(start time.Time) {
		zerolog.Ctx(h.ctx).Debug().
			Str("storage_service", h.service).
			Str("query", query).
			Any("arguments", arg).
			TimeDiff("execution_time", time.Now(), start).
			Msg("bind_named")
	}(time.Now())

	return h.ext.BindNamed(query, arg)
}

func (h handle) Rebind(query string) string {
	defer func(start time.Time) {
		zerolog.Ctx(h.ctx).Debug().
			Str("storage_service", h.service).
			Str("query", query).
			TimeDiff("execution_time", time.Now(), start).
			Msg("rebind")
	}(time.Now())

	return h.ext.Rebind(query)
}

func (h handle) DriverName() string {
	return h.ext.DriverName()
}

var _ sqlx.Execer = handle{}
var _ sqlx.Queryer = handle{}
var _ sqlx.Ext = handle{}

func (h handle) Get(dest any, query string, param any) error {
	// handle named parameters
	query, args, err := sqlx.Named(query, param)
	if err != nil {
		return err
	}

	// handle slice parameters, these get expanded to multiple inputs
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return err
	}

	// rebind the query to our database type
	query = h.ext.Rebind(query)
	return sqlx.GetContext(h.ctx, h.ext, dest, query, args...)
}

func (h handle) Select(dest any, query string, param any) error {
	// handle named parameters
	query, args, err := sqlx.Named(query, param)
	if err != nil {
		return err
	}

	// handle slice parameters, these get expanded to multiple inputs
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return err
	}

	// rebind the query to our database type
	query = h.ext.Rebind(query)
	return sqlx.SelectContext(h.ctx, h.ext, dest, query, args...)
}

package mariadb

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

var newsColumnNames = []string{
	"id", "title", "header", "body",
	"deleted_at", "created_at", "updated_at",
	"user.id", "private",
}

func compareAsJSON(t *testing.T, a, b interface{}) {
	aj, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bj, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(aj, bj) != 0 {
		t.Errorf("%s != %s", aj, bj)
	}
}

func newsPostToRow(post radio.NewsPost) []driver.Value {
	return []driver.Value{
		post.ID, post.Title, post.Header, post.Body,
		post.DeletedAt, post.CreatedAt, post.UpdatedAt,
		post.User.ID, post.Private,
	}
}

func newTestStorage(t *testing.T) (*StorageService, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open stub database: %s", err)
	}
	t.Cleanup(func() { db.Close() })

	sqldb := sqlx.NewDb(db, "mock")
	sqldb.MapperFunc(mapperFunc)

	storage := &StorageService{sqldb}
	return storage, mock
}

func TestNewsStorageGet(t *testing.T) {
	storage, mock := newTestStorage(t)
	ctx := context.Background()
	ns := storage.News(ctx)

	now := time.Now()
	expected := radio.NewsPost{
		ID:        1,
		Title:     "test",
		Header:    "a small test",
		Body:      "a small test",
		DeletedAt: &now,
		CreatedAt: now,
		UpdatedAt: &now,
		Private:   false,
	}

	rows := sqlmock.NewRows(newsColumnNames).AddRow(newsPostToRow(expected)...)
	mock.ExpectQuery("^SELECT (.+) FROM radio_news").WithArgs(1).WillReturnRows(rows)

	// first with all fields filled
	post, err := ns.Get(1)
	if err != nil {
		t.Error(err)
	}

	compareAsJSON(t, post, expected)

	// second with possible nil values set to nil
	expected.DeletedAt = nil
	expected.UpdatedAt = nil
	rows = sqlmock.NewRows(newsColumnNames).AddRow(newsPostToRow(expected)...)
	mock.ExpectQuery("^SELECT (.+) FROM radio_news").WithArgs(1).WillReturnRows(rows)

	post, err = ns.Get(1)
	if err != nil {
		t.Error(err)
	}

	compareAsJSON(t, post, expected)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfilled expectations: %s", err)
	}
}

func TestNewsStorageDelete(t *testing.T) {
	storage, mock := newTestStorage(t)
	ctx := context.Background()
	ns := storage.News(ctx)

	// normal delete
	mock.ExpectExec("UPDATE radio_news").WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// delete that failed to affect any rows
	mock.ExpectExec("UPDATE radio_news").WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := ns.Delete(1)
	if err != nil {
		t.Error(err)
	}

	err = ns.Delete(1)
	if !errors.Is(errors.InvalidArgument, err) {
		t.Errorf("failed to delete not caught by code")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfilled expectations: %s", err)
	}
}

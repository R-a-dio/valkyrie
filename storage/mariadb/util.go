package mariadb

import (
	"iter"

	"github.com/jmoiron/sqlx"
)

// SelectIter is like Select but returns an iterator instead
func SelectIter[T any](ex sqlx.Queryer, query string, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		rows, err := ex.Queryx(query, args...)
		if err != nil {
			yield(*new(T), err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var dest T

			err = rows.StructScan(&dest)
			if !yield(dest, err) {
				return
			}
		}

		if err = rows.Err(); err != nil {
			yield(*new(T), err)
			return
		}
	}
}

// Select is like sqlx.Select except with generics and only support for struct destination
func Select[T any](ex sqlx.Queryer, query string, args ...any) ([]T, error) {
	rows, err := ex.Queryx(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []T
	for rows.Next() {
		var dest T

		err = rows.StructScan(&dest)
		if err != nil {
			return nil, err
		}
		res = append(res, dest)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// Collect collects all the values in seq in a slice, if an error
// is encountered it returns (nil, err) instead.
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var res []T
	for t, err := range seq {
		if err != nil {
			return nil, err
		}

		res = append(res, t)
	}
	return res, nil
}

// Each calls function fn for every item in seq aslong as err == nil
func Each[T any](seq iter.Seq2[T, error], fn func(T) T) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for t, err := range seq {
			if err != nil {
				t = fn(t)
			}
			if !yield(t, err) {
				return
			}
		}
	}
}

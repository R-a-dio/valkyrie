// +build musly

package musly

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrMissingTrack is returned when a TrackID given does not exist
	ErrMissingTrack = errors.New("musly: missing track")
	// ErrInvalidInput is returned when invalid arguments are passed
	ErrInvalidInput = errors.New("musly: invalid input")
	// ErrUnknown is returned when an unknown error is encountered
	ErrUnknown = errors.New("musly: unknown error")
	// ErrAnalyze is returned when analyzing of a track fails
	ErrAnalyze = errors.New("musly: analyze failure")
	// ErrInvalidDatabase is returned when an invalid state has occured in the
	// bolt database
	ErrInvalidDatabase = errors.New("musly: database error")
)

// Error returned by this package that contains extra information
type Error struct {
	Err  error
	Info string
	IDs  []TrackID
}

func (e Error) Error() string {
	if e.Info != "" && len(e.IDs) > 0 {
		ids := buildString(e.IDs)
		return fmt.Sprintf("%s: %s: %s", e.Err.Error(), e.Info, ids)
	} else if e.Info != "" {
		return fmt.Sprintf("%s: %s", e.Err.Error(), e.Info)
	} else if len(e.IDs) > 0 {
		ids := buildString(e.IDs)
		return fmt.Sprintf("%s: %s", e.Err.Error(), ids)
	}

	return e.Err.Error()
}

// IsMissingTrack returns true if the error given is an ErrMissingTrack
func IsMissingTrack(err error) bool {
	e, ok := err.(*Error)
	return err == ErrMissingTrack || (ok && e.Err == ErrMissingTrack)
}

// IsInvalidInput returns true if the error given is an ErrInvalidInput
func IsInvalidInput(err error) bool {
	e, ok := err.(*Error)
	return err == ErrInvalidInput || (ok && e.Err == ErrInvalidInput)
}

// IsInvalidDatabase returns true if the error given is an ErrInvalidDatabase
func IsInvalidDatabase(err error) bool {
	e, ok := err.(*Error)
	return err == ErrInvalidDatabase || (ok && e.Err == ErrInvalidDatabase)
}

func buildString(ids []TrackID) string {
	var b strings.Builder
	for i := range ids {
		fmt.Fprintf(&b, "%d ", ids[i])
	}

	return b.String()
}

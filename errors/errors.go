package errors

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"runtime"
	"time"

	radio "github.com/R-a-dio/valkyrie"
)

// Errorf is equavalent to fmt.Errorf
var Errorf = fmt.Errorf

// New is equavalent to errors.New
var New = errors.New

// E builds an error value from its arguments.
// There must be at least one argument or E panics.
// The type of each argument determines its meaning.
// If more than one argument of a given type is presented,
// only the last one is recorded.
//
// The types are:
// 		radio.SongID:
//			The song identifier of the song involved
//		radio.TrackID:
//			The track identifier of the song involved
//		radio.Song, radio.QueueEntry:
//			The song involved, fills in both SongID and TrackID above
//		errors.Delay:
//			The delay until this error is resolved
//		errors.Info:
//			Extra info useful to this class of error, think argument
//			name when using InvalidArgument
//		errors.Op:
//			The operation being performed
//		string:
//			Treated as an error message and assigned to the
//			Err field after a call to errors.New
// 		errors.Kind:
//			The class of error
//		error:
//			The underlying error that triggered this one
//
// If the error is printed, only those items that have been
// set to non-zero values will appear in the result.
//
// If Kind is not specified or Other, we set it to the Kind of
// the underlying error.
func E(args ...interface{}) error {
	if len(args) == 0 {
		panic("call to errors.E with no arguments")
	}

	e := &Error{}
	for _, arg := range args {
		switch arg := arg.(type) {
		case Kind:
			e.Kind = arg
		case Op:
			e.Op = arg
		case Delay:
			e.Delay = arg
		case Info:
			e.Info = arg
		case radio.QueueEntry:
			e.SongID = arg.ID
			if arg.HasTrack() {
				e.TrackID = arg.TrackID
			}
		case radio.Song:
			e.SongID = arg.ID
			if arg.HasTrack() {
				e.TrackID = arg.TrackID
			}
		case radio.SongID:
			e.SongID = arg
		case radio.TrackID:
			e.TrackID = arg
		case string:
			e.Err = errors.New(arg)
		case *Error:
			copy := *arg
			e.Err = &copy
		case error:
			e.Err = arg
		default:
			_, file, line, _ := runtime.Caller(1)
			log.Printf("errors.E: bad call from %s:%d: %v", file, line, args)
			return Errorf("unknown type %T, value %v in error call", arg, arg)
		}
	}

	prev, ok := e.Err.(*Error)
	if !ok {
		return e
	}

	// The previous error was also one of ours. Suppress duplications
	// so the message won't contain the same information twice
	if prev.Kind == e.Kind {
		prev.Kind = Other
	}
	if prev.SongID == e.SongID {
		prev.SongID = 0
	}
	if prev.TrackID == e.TrackID {
		prev.TrackID = 0
	}
	if prev.Delay == e.Delay {
		prev.Delay = 0
	}
	if prev.Info == e.Info {
		prev.Info = ""
	}
	// if this error has Kind unset or Other, pull up the inner one
	if e.Kind == Other {
		e.Kind = prev.Kind
		prev.Kind = Other
	}

	return e
}

// SelectDelay returns the first non-zero Delay found in the error given, if none
// is found ok will be false
func SelectDelay(err error) (Delay, bool) {
	e, ok := err.(*Error)
	if !ok {
		return 0, false
	}

	if e.Delay != 0 {
		return e.Delay, true
	}
	if e.Err != nil {
		return SelectDelay(e.Err)
	}
	return 0, false
}

// Select returns an *Error with the given Kind from the error given
func Select(kind Kind, err error) (*Error, bool) {
	e, ok := err.(*Error)
	if !ok {
		return nil, false
	}

	if e.Kind == kind {
		return e, true
	}
	if e.Err != nil {
		return Select(kind, e.Err)
	}

	return nil, false
}

// Is reports whether err is an *Error of the given kind
func Is(kind Kind, err error) bool {
	e, ok := err.(*Error)
	if !ok {
		return false
	}
	if e.Kind != Other {
		return e.Kind == kind
	}
	if e.Err != nil {
		return Is(kind, e.Err)
	}
	return false
}

// Op is the operation that was being performed
type Op string

// Delay is the amount of time still left on a cooldown
type Delay time.Duration

// Info is some extra information that can be included with an Error
type Info string

// Error is the type that implements the error interface.
// It contains a number of fields, each of different type.
// An Error value may leave some values unset.
type Error struct {
	Kind    Kind
	Op      Op
	SongID  radio.SongID
	TrackID radio.TrackID
	Delay   Delay
	Info    Info
	Err     error
}

func (e *Error) isZero() bool {
	return e == nil || *e == Error{}
}

// pad appends s to the buffer if the buffer already contains data
func pad(b *bytes.Buffer, s string) {
	if b.Len() != 0 {
		b.WriteString(s)
	}
}

func (e *Error) Error() string {
	b := new(bytes.Buffer)

	if e.Op != "" {
		pad(b, ": ")
		b.WriteString(string(e.Op))
	}

	if e.Kind != 0 {
		pad(b, ": ")
		b.WriteString(e.Kind.String())
	}

	var hadPrevious bool
	infoPad := func() {
		if hadPrevious {
			pad(b, ", ")
		} else {
			pad(b, ": ")
		}
		hadPrevious = true
	}

	if e.SongID != 0 {
		infoPad()
		b.WriteString("SongID<")
		b.WriteString(e.SongID.String())
		b.WriteString(">")
	}

	if e.TrackID != 0 {
		infoPad()
		b.WriteString("TrackID<")
		b.WriteString(e.TrackID.String())
		b.WriteString(">")
	}

	if e.Delay != 0 {
		infoPad()
		b.WriteString("Delay<")
		b.WriteString(time.Duration(e.Delay).String())
		b.WriteString(">")
	}

	if e.Info != "" {
		infoPad()
		b.WriteString("Info<")
		b.WriteString(string(e.Info))
		b.WriteString(">")
	}

	if e.Err != nil {
		// indent on new line if we're cascading non-empty Error
		if prev, ok := e.Err.(*Error); ok && !prev.isZero() {
			pad(b, Separator)
			b.WriteString(prev.Error())
		} else {
			pad(b, ": ")
			b.WriteString(e.Err.Error())
		}
	}

	if b.Len() == 0 {
		return "no error"
	}

	return b.String()
}

// Separator is the string used to separate nested errors. By
// default, to make errors easier on the eye, nested errors are
// indented on a new line. A server may instead choose to keep each
// error on a single line by modifying the separator string, perhaps
// to ":: ".
var Separator = ":\n\t"

// Kind defines the kind of error this is
type Kind uint8

// Kinds of errors
//
// Do not reorder this list or remove items;
// New items must be added only to the end
const (
	Other                  Kind = iota // Unclassified error
	InvalidArgument                    // Invalid argument given to function
	SongUnknown                        // Song does not exist
	SongWithoutTrack                   // Song did not have required track fields
	SongUnusable                       // Song is not playable by streamer
	SongCooldown                       // Song is on request cooldown
	UserCooldown                       // User is on request cooldown
	UserUnknown                        // User does not exist
	StreamerNotRunning                 // Streamer isn't running
	StreamerRunning                    // Streamer is already running
	StreamerAlreadyStopped             // Streamer is already stopping
	StreamerNoRequests                 // Streamer requests are disabled
	QueueEmpty                         // Queue is empty
	QueueExhausted                     // Queue has no more songs
	QueueShort                         // Queue is short songs
	SearchIndexExists                  // Search index already exists
	SearchNoResults                    // Search had no results
	BrokenTopic                        // IRC channel topic broken
	TransactionBegin                   // Database begin transaction failure
	TransactionRollback                // Database rollback transaction failure
	TransactionCommit                  // Database commit transaction failure
	StorageUnknown                     // Unknown storage name used
	NotImplemented                     // Generic error indicating something is not implemented
)

func (k Kind) String() string {
	switch k {
	case Other:
		return "other error"
	case InvalidArgument:
		return "invalid argument"
	case SongUnknown:
		return "unknown song"
	case SongWithoutTrack:
		return "song misses track fields"
	case SongUnusable:
		return "song not usable"
	case SongCooldown:
		return "song is on cooldown"
	case UserCooldown:
		return "user is on cooldown"
	case UserUnknown:
		return "unknown user"
	case StreamerNotRunning:
		return "streamer is not running"
	case StreamerRunning:
		return "streamer is running"
	case StreamerAlreadyStopped:
		return "streamer is already stopping"
	case StreamerNoRequests:
		return "streamer is not taking requests"
	case QueueEmpty:
		return "empty queue"
	case QueueExhausted:
		return "exhausted queue"
	case QueueShort:
		return "short queue"
	case SearchIndexExists:
		return "search index exists"
	case SearchNoResults:
		return "no search results"
	case BrokenTopic:
		return "topic format is broken"
	case TransactionBegin:
		return "failed to begin transaction"
	case TransactionRollback:
		return "failed to rollback transaction"
	case TransactionCommit:
		return "failed to commit transaction"
	case StorageUnknown:
		return "unknown storage"
	case NotImplemented:
		return "not implemented"
	}

	return "unknown error kind"
}

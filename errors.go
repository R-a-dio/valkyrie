package radio

import "time"

// UserError is an error that includes a message suitable for the user
//
// a UserError that reaches the IRC handlers will be send to the appropiate location
type UserError interface {
	error
	Public() bool
	UserError() string
}

// ErrRequestsDisabled is returned when requests are currently disabled
var ErrRequestsDisabled = createSRE("requests are currently disabled")

// ErrUnusableSong is returned when a song can't be played by the streamer
var ErrUnusableSong = createSRE("this song can't be requested")

// ErrSongCooldown is returned when a song is still on cooldown
var ErrSongCooldown = createSRE("you need to wait longer before requesting this song")

// ErrUserCooldown is returned when the identifier given still has a cooldown
var ErrUserCooldown = createSRE("you need to wait longer before requesting again")

// ErrUnknownSong is returned when the song is unknown to the streamer
var ErrUnknownSong = createSRE("unknown song requested")

// ErrInvalidRequest is returned when the arguments given are invalid
var ErrInvalidRequest = createSRE("error: please contact a site admin")

func createSRE(s string) SongRequestError {
	return SongRequestError{UserMessage: s}
}

// SongRequestError is returned by a StreamerService.RequestSong when the request failed
// to be requested due to cooldowns or other criteria
type SongRequestError struct {
	// UserMessage is a message suitable for outside users, describing why the request
	// was rejected
	UserMessage string
	UserDelay   time.Duration
	SongDelay   time.Duration
}

func (err SongRequestError) Error() string {
	return err.UserMessage
}

// Public implements ircbot.UserError
func (err SongRequestError) Public() bool {
	return true
}

// UserError implements ircbot.UserError
func (err SongRequestError) UserError() string {
	return err.UserMessage
}

// IsCooldownError tells you if the error given is either an ErrSongCooldown or
// ErrUserCooldown. This indicates one of the delay fields is populated
func IsCooldownError(err error) bool {
	uerr, ok := err.(SongRequestError)
	if !ok {
		return false
	}
	return uerr.UserMessage == ErrSongCooldown.UserMessage ||
		uerr.UserMessage == ErrUserCooldown.UserMessage
}

// NewStreamerError returns a new error with the arguments given
func NewStreamerError(msg string, public bool) error {
	return StreamerError{
		Message: msg,
		Private: !public,
	}
}

// StreamerError is returned by the streamer when something goes wrong
type StreamerError struct {
	Message string
	Private bool
}

func (err StreamerError) Error() string {
	return err.Message
}

// Public implements ircbot.UserError
func (err StreamerError) Public() bool {
	return !err.Private
}

// UserError implements ircbot.UserError
func (err StreamerError) UserError() string {
	return err.Message
}

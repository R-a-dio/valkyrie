package radio

// SongRequestError is returned by a StreamerService.RequestSong when the request failed
// to be requested due to cooldowns or other criteria
type SongRequestError struct {
	// UserMessage is a message suitable for outside users, describing why the request
	// was rejected
	UserMessage string
}

func (err SongRequestError) Error() string {
	return err.UserMessage
}

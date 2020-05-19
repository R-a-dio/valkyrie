package mariadb

import radio "github.com/R-a-dio/valkyrie"

type SessionStorage struct {
	handle handle
}

func (ss SessionStorage) Delete(token radio.SessionToken) error {
	return nil
}

func (ss SessionStorage) Save(session radio.Session) error {
	return nil
}

func (ss SessionStorage) Get(token radio.SessionToken) (radio.Session, error) {
	return radio.Session{}, nil
}

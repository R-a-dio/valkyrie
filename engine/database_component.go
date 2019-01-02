package engine

import "github.com/jmoiron/sqlx"

// DatabaseComponent connects to the database configured and adds it to the engine
func DatabaseComponent(e *Engine) (DeferFn, error) {
	var conf = e.Conf().Database
	var err error

	e.DB, err = sqlx.Connect(conf.DriverName, conf.DSN)
	if err != nil {
		return nil, err
	}
	return e.DB.Close, nil
}

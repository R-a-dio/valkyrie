package config

// DefaultDatabase contains the default for the database configuration
var DefaultDatabase = Database{
	DriverName: "mysql",
}

// Database is the configuration for the database/sql package
type Database struct {
	DriverName string
	DSN        string
}

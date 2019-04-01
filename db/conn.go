// Package db defines functions and types for interfacing with a postgres database. It exposes
// a global connection at the package level for ease-of access.
package db

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"
)

const (
	envVarDBName     = "POSTGRES_DB"
	envVarDBUser     = "POSTGRES_USER"
	envVarDBHostname = "POSTGRES_HOSTNAME"
	envVarDBPassword = "POSTGRES_PASSWORD"
)

// GlobalConn is a package level global used for connecting to a postgres database and
// is automatically configured on import from the environment in the init function.
var (
	GlobalConn = &Connection{}
)

func init() {
	getEnv := func(e string) string {
		v, exists := os.LookupEnv(e)
		if !exists {
			log.Printf("variable not defined in the current environment: %s", e)
		}
		return v
	}

	GlobalConn.Username = getEnv(envVarDBUser)
	GlobalConn.DBName = getEnv(envVarDBName)
	GlobalConn.Hostname = getEnv(envVarDBHostname)
	GlobalConn.Password = getEnv(envVarDBPassword)
}

// Connection wraps an instance of sql.DB and connection parameters.
type Connection struct {
	DBName   string
	Hostname string
	Username string
	Password string
	Port     string // currently not used

	*sql.DB
}

// Establish tries to establish a database connection. It will retry 'retryCount' number of times
// waiting 'retryCooldownSeconds' between each attempt.
func (dbc *Connection) Establish(retryCount int, retryCooldownSeconds int) error {
	wait := func() { time.Sleep(time.Duration(retryCooldownSeconds) * time.Second) } // should really use exponential backoff
	timeout := func(n int) bool { return n >= retryCount }
	attempts := 0

	if dbc.DB != nil { // first check to see if the DB is already up
		err := dbc.Ping()
		if err == nil {
			return nil
		}
	}

	for {
		conn, err := sql.Open("postgres", dbc.ConnectString())
		if err != nil {
			if timeout(attempts) {
				return err
			}

			attempts++

			log.Printf("db connection attempt: %d", attempts)

			wait()

			continue
		}

		dbc.DB = conn

		if err := dbc.Ping(); err != nil {
			if timeout(attempts) {
				return err
			}

			attempts++

			log.Printf("db ping attempt: %d", attempts)

			wait()

			continue
		}

		break
	}

	return nil
}

// ExecFrom executes SQL statements in a file where each statement is delimited by a ';'.
func (dbc *Connection) ExecFrom(filepath string) error {
	raw, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}

	cmds := strings.Split(string(raw), ";")
	for _, cmd := range cmds {
		statement, err := dbc.Prepare(cmd)
		if err != nil {
			return err
		}
		_, err = statement.Exec()
		if err != nil {
			return err
		}
		statement.Close()
	}

	return nil
}

// ConnectString formats connection parameters into a string used to connect to a postgres database.
func (dbc *Connection) ConnectString() string {
	return fmt.Sprintf(
		"host=%s user=%s dbname=%s password=%s sslmode=disable",
		dbc.Hostname,
		dbc.Username,
		dbc.DBName,
		dbc.Password)
}

package db

import (
	"testing"
)

func TestInitialiseDB(t *testing.T) {
	const (
		maxNumRetries    = 3
		retryIntervalSec = 1
	)

	if err := GlobalConn.Establish(maxNumRetries, retryIntervalSec); err != nil {
		t.Error(err)
	}

	GlobalConn.Close()
}

func TestExececuteStatementsFromAFile(t *testing.T) {
	t.Skip("not implemented")

	if err := GlobalConn.ExecFrom(""); err != nil {
		t.Error(err)
	}
}

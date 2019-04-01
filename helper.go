package main

import (
	"encoding/json"
)

func stringify(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

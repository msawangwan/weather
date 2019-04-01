package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/msawangwan/weather/db"
)

const (
	envVarListenAddr = "LISTEN_ADDR"
	envVarListenPort = "LISTEN_PORT"
)

func main() {
	var (
		ready = make(chan bool, 1)
	)

	const (
		maxNumRetries    = 10
		retryIntervalSec = 2
	)

	go func() { // spin up the db concurrently so we can complete other setup
		if err := db.GlobalConn.Establish(maxNumRetries, retryIntervalSec); err != nil {
			log.Fatal(err)
		}

		if err := db.GlobalConn.ExecFrom("./data/init.db.sql"); err != nil {
			log.Fatal(err)
		}

		log.Printf("db connection established")

		ready <- true
	}()

	defer db.GlobalConn.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/account/user", GetAccountUserInfo)
	mux.HandleFunc("/api/v1/account/user/register", CreateNewAccount)
	mux.HandleFunc("/api/v1/account/user/bookmark", AccountBookmarksCollectionAction)
	mux.HandleFunc("/api/v1/location/weather", ReportLocationWeather)
	mux.HandleFunc("/api/v1/location/weather/stats", ReportWeatherStatistics)
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) { sendMessage(w, "ok") })

	addr, _ := os.LookupEnv(envVarListenAddr)
	port, _ := os.LookupEnv(envVarListenPort)

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%s", addr, port),
		Handler: mux,
	}

	<-ready // wait for db

	log.Printf("server listening for incoming requests @ %s:%s", addr, port)
	log.Fatal(server.ListenAndServe())
}

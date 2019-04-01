package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msawangwan/weather/api"
	"github.com/msawangwan/weather/db"
)

const (
	dataDirPath = "test/data/"
)

type mockAPIServer struct {
	*httptest.Server
}

func (s *mockAPIServer) setup() error {
	wd, _ := os.Getwd()
	base := strings.Split(wd, "/")
	if base[len(base)-1] != dataDirPath {
		if err := os.Chdir(dataDirPath); err != nil {
			return err
		}
	}

	responseJSON := map[string][]byte{}

	err := filepath.Walk(wd, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(p, ".json") == false {
			return nil
		}

		raw, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}

		responseJSON[info.Name()] = raw

		return nil
	})
	if err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		params, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(params) == 0 {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resource := strings.ToLower(params["q"][0]) + ".json"

		// todo: handle locations with spaces, ie: "san francisco"
		if data, exists := responseJSON[resource]; exists {
			p := &api.Location{}
			json.Unmarshal(data, p)
			json.NewEncoder(w).Encode(&p)
			return
		}

		m := map[string]interface{}{}
		data := responseJSON["404.json"]
		json.Unmarshal(data, m)
		json.NewEncoder(w).Encode(&m)
	})

	s.Server = httptest.NewServer(mux)

	return nil
}

func (s *mockAPIServer) teardown() {
	s.Close()
}

type mockServer struct {
	*httptest.Server
}

func (s *mockServer) setup() error {
	const (
		maxNumRetries    = 3
		retryIntervalSec = 1
	)

	if err := db.GlobalConn.Establish(maxNumRetries, retryIntervalSec); err != nil {
		return err
	}

	if err := db.GlobalConn.ExecFrom("/src/data/init.db.sql"); err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/account/user", GetAccountUserInfo)
	mux.HandleFunc("/api/v1/account/user/register", CreateNewAccount)
	mux.HandleFunc("/api/v1/account/user/bookmark", AccountBookmarksCollectionAction)
	mux.HandleFunc("/api/v1/location/weather", ReportLocationWeather)
	mux.HandleFunc("/api/v1/location/weather/stats", ReportWeatherStatistics)
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) { sendMessage(w, "ok") })

	s.Server = httptest.NewServer(mux)

	return nil
}

func (s *mockServer) teardown() {
	s.Close()
	db.GlobalConn.Close()
}

type mockClient struct {
	*http.Client
	*bytes.Buffer
}

func newMockClient() *mockClient {
	c := &http.Client{}
	b := &bytes.Buffer{}

	return &mockClient{c, b}
}

type testContext struct {
	*mockAPIServer
	*mockServer
	c    *mockClient
	root string
}

func (s *testContext) setup() error {
	s.root, _ = os.Getwd()

	s.mockAPIServer = &mockAPIServer{}
	if err := s.mockAPIServer.setup(); err != nil {
		return err
	}

	s.mockServer = &mockServer{}
	if err := s.mockServer.setup(); err != nil {
		return err
	}

	s.c = newMockClient()
	api.SharedClient.APIEndpoint = strings.Split(s.mockAPIServer.URL, "//")[1]

	return nil
}

func (s *testContext) teardown() error {
	s.mockAPIServer.Close()
	s.mockServer.Close()
	if err := os.Chdir(s.root); err != nil {
		return err
	}
	return nil
}

var (
	resources = []string{
		"/api/v1/location/weather?city=Reno",
		"/api/v1/location/weather?city=London",
		"/api/v1/location/weather?city=Budapest",
	}
)

func score(t *testing.T, have, want interface{}, test func() bool) {
	if test() {
		return
	}
	t.Logf("have: %v want: %v", have, want)
	t.Fail()
}

func TestHandlers(t *testing.T) {
	initState := func() (*testContext, func()) {
		ctx := &testContext{}
		if err := ctx.setup(); err != nil {
			t.Error(err)
		}
		for _, resource := range resources {
			res, err := ctx.c.Get(ctx.mockServer.URL + resource)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != 200 {
				t.Fatal(err)
			}
		}
		return ctx, func() { ctx.teardown() }
	}

	context, cleanup := initState()
	defer cleanup()

	locationQuery := struct {
		CityName   string    `json:"city_name,omitempty"`
		Conditions []string  `json:"conditions,omitempty"`
		MedianTemp float64   `json:"median_temp,omitempty"`
		AtTime     time.Time `json:"at_time,omitempty"`
	}{
		"", []string{}, 0.0, time.Now(),
	}

	var getLocationTestCases = []struct {
		label    string
		resource string
		want     string
	}{
		{"expected location city name 1", "/api/v1/location/weather?city=Reno", "Reno"},
		{"expected location city name 2", "/api/v1/location/weather?city=London", "London"},
		// add edge cases here ..
	}

	for _, tc := range getLocationTestCases {
		t.Run(tc.label, func(t *testing.T) {
			res, err := context.c.Get(context.mockServer.URL + tc.resource)
			if err != nil {
				t.Fatal(err)
			}

			locationQuery.CityName = ""

			context.c.ReadFrom(res.Body)
			res.Body.Close()
			json.Unmarshal(context.c.Bytes(), &locationQuery)
			context.c.Buffer.Reset()

			score(t, locationQuery.CityName, tc.want, func() bool {
				return locationQuery.CityName == tc.want
			})
		})
	}

	var getLocationTempTestCases = []struct {
		label    string
		resource string
		want     float64
	}{
		{"expected location median temp 1", "/api/v1/location/weather?city=Reno", 276.2050018310547},
		{"expected location median temp 2", "/api/v1/location/weather?city=London", 280.1499938964844},
		// add edge cases here ..
	}

	for _, tc := range getLocationTempTestCases {
		t.Run(tc.label, func(t *testing.T) {
			res, err := context.c.Get(context.mockServer.URL + tc.resource)
			if err != nil {
				t.Fatal(err)
			}

			locationQuery.MedianTemp = 0.0

			context.c.ReadFrom(res.Body)
			res.Body.Close()
			json.Unmarshal(context.c.Bytes(), &locationQuery)
			context.c.Buffer.Reset()

			score(t, locationQuery.MedianTemp, tc.want, func() bool {
				return locationQuery.MedianTemp == tc.want
			})
		})
	}

	var monthlyAvgTestCases = []struct {
		label    string
		resource string
		want     float64
	}{
		{"expected averages", "/api/v1/location/weather/stats?temp=avgs", 294.92999267578125},
		// add edge cases here ..
	}

	for _, tc := range monthlyAvgTestCases {
		t.Run(tc.label, func(t *testing.T) {
			res, err := context.c.Get(context.mockServer.URL + tc.resource)
			if err != nil {
				t.Fatal(err)
			}

			statsQuery := map[string]interface{}{}

			context.c.ReadFrom(res.Body)
			res.Body.Close()
			json.Unmarshal(context.c.Bytes(), &statsQuery)
			context.c.Buffer.Reset()

			avgs := statsQuery["temperatures"].(map[string]interface{})["avgs"]

			score(t, avgs, tc.want, func() bool {
				return avgs != nil
			})
		})
	}

	// add more tests here ..
}

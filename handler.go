package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/msawangwan/weather/api"
	"github.com/msawangwan/weather/db"
)

const (
	cacheTTLMinutes = 1
)

var (
	errMethodMustBeGET       = errors.New("HTTP method must be GET")
	errMethodMustBePOST      = errors.New("HTTP method must be POST")
	errMethodMustBeGETorPOST = errors.New("HTTP method must be GET or POST")
)

// ReportLocationWeather handles GET requests for location weather. The location should be
// specified by the query parameter 'cityname'.
func ReportLocationWeather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodError(w, errMethodMustBeGET)
		return
	}

	// this *should* be an external function ..otherwise we create a new anonymous instance every request
	parseRows := func(q db.QueryResult) (lr *db.LocationRow, wr *db.WeatherRow) {
		for _, v := range q {
			switch row := v.(type) {
			case *db.LocationRow:
				lr = row
			case *db.WeatherRow:
				wr = row
			}
		}
		return lr, wr
	}

	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		internalServerError(w, err)
		return
	}

	cityName := strings.Title(params.Get("city"))
	refresh := true

	query, err := db.FetchLocationWeather(cityName)
	if err != nil {
		internalServerError(w, err)
		return
	}

	var (
		lr *db.LocationRow
		wr *db.WeatherRow
	)

	lr, wr = parseRows(query)

	if lr != nil && wr != nil {
		delta := time.Now().Sub(wr.AtTime)

		if delta.Minutes() < cacheTTLMinutes {
			refresh = false

			if err := lr.IncrQueryCount(); err != nil {
				internalServerError(w, err)
				return
			}
		}
	}

	if refresh {
		location, err := api.SharedClient.FetchCurrentWeatherByLocationName(cityName)
		if err != nil {
			internalServerError(w, err)
			return
		}

		if location.Cod != 200 {
			if location.Message != nil {
				sendMessage(w, *location.Message)
			} else {
				sendMessage(w, "failed to communicate with the openweather api: unknown reason")
			}
			return
		}

		tempMin := location.Main.TempMin
		tempMax := location.Main.TempMax
		labels := location.WeatherLabels()

		query, err = db.UpdateCachedLocationWeather(cityName, tempMin, tempMax, labels...)
		if err != nil {
			internalServerError(w, err)
			return
		}

		lr, wr = parseRows(query)
	}

	sendJSON(
		w,
		struct {
			CityName   string    `json:"city_name,omitempty"`
			Conditions []string  `json:"conditions,omitempty"`
			LowTemp    float64   `json:"low_temp,omitempty"`
			HighTemp   float64   `json:"high_temp,omitempty"`
			MedianTemp float64   `json:"median_temp,omitempty"`
			AtTime     time.Time `json:"at_time,omitempty"`
		}{
			cityName,
			wr.Labels,
			wr.TempLow.Float64,
			wr.TempHigh.Float64,
			(wr.TempLow.Float64 + wr.TempHigh.Float64) / 2, // uh-oh, overflow (jk, unlikely but this would be somthing to test huh)
			wr.AtTime,
		})
}

// ReportWeatherStatistics handles GET requests for various weather stats depending
// on what query parameter are set. If no query string is found in the uri, the full list of
// available parameters is returned as a JSON payload.
func ReportWeatherStatistics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodError(w, errMethodMustBeGET)
		return
	}

	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		internalServerError(w, err)
		return
	}

	// if no query parameters are attached to the request, we send a payload that lists available query parameters
	if sendDoc(w,
		struct {
			ValidQueryParameters []string
		}{
			[]string{
				"count=query|labels (only query is implemented)",
				"summary=day|month|year (only day is implemented)",
				"temp=lows|highs|avgs",
			},
		},
		func() bool { return len(params) == 0 },
	) {
		return
	}

	var (
		stats = make(map[string]interface{})
	)

	for q, p := range params {
		switch q {
		case "count":
			if hasParam(p, "query") {
				count, err := db.TotalQueryCount()
				if err != nil {
					internalServerError(w, err)
					return
				}

				stats["count"] = map[string]interface{}{
					"location_queries": &count,
				}
			}

			if hasParam(p, "labels") {
				labels, err := db.KnownWeatherLabels()
				if err != nil {
					internalServerError(w, err)
					return
				}

				stats["labels"] = labels
			}

			break
		case "summary":
			if hasParam(p, "day") {
				summary, err := db.DailyWeatherSummary()
				if err != nil {
					internalServerError(w, err)
					return
				}

				stats["summary"] = map[string]interface{}{
					"daily": summary,
				}
			}

			break
		case "temp":
			if hasParam(p, "lows", "highs", "avgs") { // can get lows, highs and avgs in one query
				temps := map[string]db.LocationTemperatureQueryResult{}

				for _, subv := range p {
					f := db.TemperatureQueryFilter(subv)

					var report db.LocationTemperatureQueryResult

					if f == db.FilterAverages {
						report, err = db.MonthlyAverageTemperature()
					} else {
						report, err = db.MonthlyTemperature(f)
					}

					if err != nil {
						internalServerError(w, err)
						return
					}

					temps[subv] = report
				}

				stats["temperatures"] = temps
			}

			break
		}
	}

	sendJSON(w, stats)
}

// GetAccountUserInfo handles GET requests for account user info. The account user
// should be specifed by as a value to the query parameter 'username'.
func GetAccountUserInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodError(w, errMethodMustBeGET)
		return
	}

	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		internalServerError(w, err)
		return
	}

	username := params.Get("username")

	acc, err := db.ExistingAccount(username)
	if err != nil {
		internalServerError(w, err)
		return
	}

	if acc == nil {
		sendMessage(
			w, "no account found with that username: "+username)
		return
	}

	sendJSON(w, struct {
		Name string `json:"name,omitempty"`
		ID   int64  `json:"id,omitempty"`
	}{
		acc.Name.String,
		acc.ID.Int64,
	})
}

// CreateNewAccount handles POST requests for registering a new account. Clients
// must send the account username in a JSON payload, for example: {"username": str}.
func CreateNewAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodError(w, errMethodMustBePOST)
		return
	}

	payload := map[string]string{}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		internalServerError(w, err)
		return
	}

	username := payload["username"]

	acc, err := db.NewAccount(username)
	if err != nil {
		internalServerError(w, err)
		return
	}

	col, err := acc.NewBookmarkCollection()
	if err != nil {
		internalServerError(w, err)
		return
	}

	sendJSON(
		w,
		struct {
			Name                  string  `json:"name,omitempty"`
			ID                    int64   `json:"id,omitempty"`
			BookmarkCollectionID  int64   `json:"bookmark_collection_id,omitempty"`
			BookmarkedLocationIDs []int64 `json:"bookmarked_location_i_ds,omitempty"`
		}{
			acc.Name.String,
			acc.ID.Int64,
			col.ID.Int64,
			col.LocationIDs,
		})
}

// AccountBookmarksCollectionAction handles bot GET and POST requests. As a GET, returns
// the bookmarks for an account user, where the account user is specified as the query parameter, 'username'.
// As a POST, will update the bookmarks of an account user where the username and bookmarks to be added
// is specified by the JSON payload: {"username": str, "locations": str[]}
func AccountBookmarksCollectionAction(w http.ResponseWriter, r *http.Request) {
	var (
		result interface{}
	)

	switch r.Method {
	case http.MethodGet:
		params, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			internalServerError(w, err)
			return
		}

		username := params.Get("username")

		acc, err := db.ExistingAccount(username)
		if err != nil {
			internalServerError(w, err)
			return
		}

		if acc == nil {
			sendMessage(
				w, "no account found with that username: "+username)
			return
		}

		col, err := acc.GetBookmarkCollectionIDs()
		if err != nil {
			internalServerError(w, err)
			return
		}

		if col == nil {
			sendMessage(
				w, fmt.Sprintf("no bookmark collection associated with that id: %d", col.ID.Int64))
			return
		}

		locs, err := col.NamesFromIDs()
		if err != nil {
			internalServerError(w, err)
			return
		}

		result = struct {
			Bookmarks []string
		}{
			locs,
		}

		break
	case http.MethodPost:
		payload := struct {
			Username  string
			Locations []string
		}{
			"",
			[]string{},
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			internalServerError(w, err)
			return
		}

		acc, err := db.ExistingAccount(payload.Username)
		if err != nil {
			internalServerError(w, err)
			return
		}

		if acc == nil {
			sendMessage(
				w, "no account found with that username: "+payload.Username)
			return
		}

		newIDs, err := db.IDsFromNames(payload.Locations...)
		if err != nil {
			internalServerError(w, err)
			return
		}

		col, err := acc.UpdateBookmarkCollectionIDs(newIDs...)
		if err != nil {
			internalServerError(w, err)
			return
		}

		if col == nil {
			sendMessage(
				w, fmt.Sprintf("no bookmark collection associated with that id: %d", col.ID.Int64))
			return
		}

		locs, err := col.NamesFromIDs()
		if err != nil {
			internalServerError(w, err)
			return
		}

		result = struct {
			Bookmarks []string
		}{
			locs,
		}
	default:
		methodError(w, errMethodMustBeGETorPOST)
		return
	}

	sendJSON(w, result)
}

/*
	utility functions
*/

func hasParam(p []string, targets ...string) bool {
	if len(p) > 0 {
		// this comparison is a potential attack surface?
		for _, v := range p {
			for _, t := range targets {
				if strings.ToLower(v) == t {
					return true
				}
			}
		}
	}
	return false
}

func sendJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("content-type", "application/json")
	log.Println("\n", stringify(payload))
	json.NewEncoder(w).Encode(payload)
}

func sendMessage(w http.ResponseWriter, m string) {
	sendJSON(w, struct {
		Message string `json:"message,omitempty"`
	}{
		m,
	})
}

func sendDoc(w http.ResponseWriter, doc interface{}, pred func() bool) bool {
	if pred() {
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(doc)

		return true
	}

	return false
}

func methodError(w http.ResponseWriter, er error) {
	http.Error(w, er.Error(), http.StatusMethodNotAllowed)
}

func internalServerError(w http.ResponseWriter, er error) {
	log.Println(er)
	http.Error(w, er.Error(), http.StatusInternalServerError)
}

// Package api defines functions and types for interfacing with the openweather api. It exposes
// a package level global for ease-of use.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
)

type weather struct {
	ID          int    `json:"id,omitempty"`
	Label       string `json:"main,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type conditions struct {
	Temp     float64 `json:"temp,omitempty"`
	Pressure float64 `json:"pressure,omitempty"`
	Humidity float64 `json:"humidity,omitempty"`
	TempMin  float64 `json:"temp_min,omitempty"`
	TempMax  float64 `json:"temp_max,omitempty"`
}

type wind struct {
	Speed float64 `json:"speed,omitempty"`
	Deg   float64 `json:"deg,omitempty"`
}

type clouds struct {
	All int `json:"all,omitempty"`
}

type coordinate struct {
	Lon float64 `json:"lon,omitempty"`
	Lat float64 `json:"lat,omitempty"`
}

// Location represents a JSON payload returned by an openweather api call.
type Location struct {
	Name string `json:"name,omitempty"`
	Base string `json:"base,omitempty"`

	ID         int `json:"id,omitempty"`
	Visibility int `json:"visibility,omitempty"`
	Cod        int `json:"cod,omitempty"`

	Weather []*weather  `json:"weather,omitempty"`
	Main    *conditions `json:"main,omitempty"`
	Wind    *wind       `json:"wind,omitempty"`
	Clouds  *clouds     `json:"clouds,omitempty"`

	Coord *coordinate `json:"coord,omitempty"`

	Message *string `json:"message,omitempty"`
}

// WeatherLabels returns all the different weather types at a location
func (l *Location) WeatherLabels() []string {
	labels := []string{}
	for _, w := range l.Weather {
		labels = append(labels, w.Label)
	}

	return labels
}

// OpenWeather is used for making calls to the openweather api. Configuration options
// are loaded from the JSON file under 'config/api.json'.
type OpenWeather struct {
	APIKey      string `json:"api_key,omitempty"`
	APIEndpoint string `json:"api_endpoint,omitempty"`
}

// FetchCurrentWeatherByLocationName returns an initialised Location struct, populated
// from the results of querying the openweather api.
func (o *OpenWeather) FetchCurrentWeatherByLocationName(name string) (*Location, error) {
	resource, err := url.Parse(fmt.Sprintf("http://%s/weather", o.APIEndpoint))
	if err != nil {
		return nil, err
	}

	query := resource.Query()

	query.Set("q", name)
	query.Set("appid", o.APIKey)

	resource.RawQuery = query.Encode()

	res, err := http.Get(resource.String())
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	b.ReadFrom(res.Body)
	res.Body.Close()

	var loc *Location

	if err := json.Unmarshal(b.Bytes(), &loc); err != nil {
		return nil, err
	}

	return loc, nil
}

// Exported environment variable keys that are expected to exists in the current
// environment.
const (
	envVarAPIKey      = "API_KEY"
	envVarAPIEndpoint = "API_ENDPOINT"
)

// SharedClient is a package level global that can be used for calling the
// openweather api. It is automatically configured on package import in init from variables
// defined in the current environment.
var (
	SharedClient = &OpenWeather{}
)

func init() {
	getEnv := func(e string) string {
		v, exists := os.LookupEnv(e)
		if !exists {
			log.Printf("variable not defined in the current environment: %s", e)
		}
		return v
	}

	SharedClient.APIKey = getEnv(envVarAPIKey)
	SharedClient.APIEndpoint = getEnv(envVarAPIEndpoint)
}

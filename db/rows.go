package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
)

// QueryResult is a short-hand alias for the result of a query that will eventually
// be sent to a client as JSON.
type QueryResult map[string]interface{}

// QueryResultList is simply a type alias for convenience.
type QueryResultList map[string][]interface{}

// LocationRow represents a database row in the 'locations' table.
type LocationRow struct {
	ID         sql.NullInt64
	QueryCount sql.NullInt64
	CityName   sql.NullString
}

// IncrQueryCount increments a counter for the location in the 'locations' table
// each time it is queried for weather.
func (lr *LocationRow) IncrQueryCount() error {
	lr.QueryCount.Int64 = lr.QueryCount.Int64 + int64(1)

	query := `
		update locations
			set query_count = $2
		where
			city_name = $1`

	stmt, err := GlobalConn.Prepare(query)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(lr.CityName, lr.QueryCount)
	if err != nil {
		return err
	}

	return stmt.Close()
}

// WeatherRow represents a database row in the 'weather' table.
type WeatherRow struct {
	LocationRowID sql.NullInt64
	TempHigh      sql.NullFloat64
	TempLow       sql.NullFloat64
	Labels        pq.StringArray
	AtTime        time.Time
}

// FetchLocationWeather returns a join of the 'locations' and 'weather' table from the database for
// a row matching 'cityName'.
func FetchLocationWeather(cityName string) (QueryResult, error) {
	query := `
		select
			id,
			city_name,
			query_count,
			location_id,
			labels,
			temp_high,
			temp_low,
			at_time
		from
			locations, weather
		where
			city_name = $1
			and at_time = (select max(at_time) from weather)`

	lr := &LocationRow{}
	wr := &WeatherRow{}

	row := GlobalConn.QueryRow(query, cityName)

	switch err := row.Scan(
		&lr.ID,
		&lr.CityName,
		&lr.QueryCount,
		&wr.LocationRowID,
		&wr.Labels,
		&wr.TempHigh,
		&wr.TempLow,
		&wr.AtTime); err {
	case sql.ErrNoRows:
		return nil, nil
	case err:
		return nil, err
	default:
		return QueryResult{
			"location": lr,
			"weather":  wr,
		}, nil
	}
}

// UpdateCachedLocationWeather will update the cached weather for a location in the 'weather' table.
func UpdateCachedLocationWeather(cityName string, tempMin, tempMax float64, labels ...string) (QueryResult, error) {
	var (
		query string
		stmt  *sql.Stmt
		row   *sql.Row
		err   error
	)

	txn, txnError := GlobalConn.Begin()
	if txnError != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			txn.Rollback()
			return
		}

		err = txn.Commit()
	}()

	query = `
		insert into locations (city_name, query_count)
			values ($1, $2)
		on conflict (city_name) do
			update
				set query_count = locations.query_count + 1
		returning
			id, city_name, query_count`

	stmt, err = txn.Prepare(query)
	if err != nil {
		return nil, err
	}

	lr := &LocationRow{}

	row = stmt.QueryRow(cityName, 1)
	if err = row.Scan(&lr.ID, &lr.CityName, &lr.QueryCount); err != nil {
		return nil, err
	}

	stmt.Close()

	query = `
		insert into weather (location_id, labels, temp_low, temp_high, at_time)
			values ($1, $2, $3, $4, $5)
		returning
			location_id, labels, temp_high, temp_low, at_time`

	stmt, err = txn.Prepare(query)
	if err != nil {
		return nil, err
	}

	wr := &WeatherRow{}

	row = stmt.QueryRow(lr.ID, pq.StringArray(labels), tempMin, tempMax, time.Now())
	if err := row.Scan(
		&wr.LocationRowID,
		&wr.Labels,
		&wr.TempHigh,
		&wr.TempLow,
		&wr.AtTime); err != nil {
		return nil, err
	}

	stmt.Close()

	return QueryResult{
		"location": lr,
		"weather":  wr,
	}, nil
}

// TotalQueryCount returns the sum total of all the counts for each cached location.
func TotalQueryCount() (int, error) {
	query := `
		select sum(query_count)
			from locations
		where city_name is not null`

	var count sql.NullInt64

	if err := GlobalConn.QueryRow(query).Scan(&count); err != nil {
		return 0, err
	}

	if count.Valid {
		return int(count.Int64), nil
	}

	return 0, nil
}

// KnownWeatherLabels returns a list of unique weather label types cached in the database.
func KnownWeatherLabels() ([]string, error) {
	query := `
		select labels
			from weather
		where location_id is not null`

	rows, err := GlobalConn.Query(query)
	if err != nil {
		return nil, err
	}

	labels := []string{}
	uniqueLabels := map[string]bool{}

	for rows.Next() {
		ls := []string{}

		if err := rows.Scan(pq.Array(&ls)); err != nil {
			return nil, err
		}

		for _, l := range ls {
			if _, seen := uniqueLabels[l]; seen {
				continue
			}

			uniqueLabels[l] = true

			labels = append(labels, l)
		}
	}

	return labels, nil
}

// DailyWeatherSummary returns each unique weather label type as keys mapped to a list
// of locations where that weather type was seen.
func DailyWeatherSummary() (QueryResultList, error) {
	query := `
		select
			locations.city_name,
			locations.id,
			weather.at_time,
			weather.labels,
			weather.location_id
		from locations, weather
		where
			locations.city_name is not null
			and locations.id = weather.location_id
		order by weather.at_time desc`

	rows, err := GlobalConn.Query(query)
	if err != nil {
		return nil, err
	}

	summary := QueryResultList{}

	for rows.Next() {
		var (
			c   sql.NullString
			id  sql.NullInt64
			lid sql.NullInt64
		)

		t := time.Time{}
		ls := []string{}

		if err := rows.Scan(&c, &id, &t, pq.Array(&ls), &lid); err != nil {
			return nil, err
		}

		if !c.Valid {
			continue
		}

		for _, l := range ls {
			if _, initialised := summary[l]; !initialised {
				summary[l] = []interface{}{}
			}

			summary[l] = append(
				summary[l],
				struct {
					CityName string
					Date     time.Time
				}{
					c.String,
					t,
				})
		}
	}

	return summary, nil
}

// TemperatureQueries is a type alias for convenience, represents a list of temperatures.
type TemperatureQueries []float64

// DailyTemperatureQuery is another convenience type alias.
type DailyTemperatureQuery map[int]TemperatureQueries

// MonthlyTemperatureQuery is another convenience type alias.
type MonthlyTemperatureQuery map[int]DailyTemperatureQuery

// YearlyTemperatureQuery is another convenience type alias.
type YearlyTemperatureQuery map[int]MonthlyTemperatureQuery

// LocationTemperatureQueryResult is another convenience type alias. It represents
// a location - day/month/year JSON structure.
type LocationTemperatureQueryResult map[string]YearlyTemperatureQuery

// InitialiseForDate is a helper function for ensuring that a key for the specified date is referencing an initialised map.
func (q LocationTemperatureQueryResult) InitialiseForDate(city string, y int, mo int, d int) {
	if _, initialised := q[city]; !initialised {
		q[city] = YearlyTemperatureQuery{}
	}
	if _, initialised := q[city][y]; !initialised {
		q[city][y] = MonthlyTemperatureQuery{}
	}

	if _, initialised := q[city][y][mo]; !initialised {
		q[city][y][mo] = DailyTemperatureQuery{}
	}

	if _, initialised := q[city][y][mo][d]; !initialised {
		q[city][y][mo][d] = TemperatureQueries{}
	}
}

// Add appends a temperature to the list of temperatures at day/month/year for a location.
func (q LocationTemperatureQueryResult) Add(temp float64, city string, y int, mo int, d int) {
	q[city][y][mo][d] = append(q[city][y][mo][d], temp)
}

// TemperatureQueryFilter is a string constant that defines the available filters
// for querying temperature statistics.
type TemperatureQueryFilter string

// Exported temperature statistics query filter enums
const (
	FilterLows     TemperatureQueryFilter = "lows"
	FilterHighs    TemperatureQueryFilter = "highs"
	FilterAverages TemperatureQueryFilter = "avgs"
)

// MonthlyTemperature returns location temperature metrics based on the given filter. Currently
// only supports 'FilterLows' and 'FilterHighs'.
func MonthlyTemperature(f TemperatureQueryFilter) (LocationTemperatureQueryResult, error) {
	param := "temp_low"

	switch f {
	case FilterLows:
		break
	case FilterHighs:
		param = "temp_high"
	default:
		return nil, fmt.Errorf("invalid reporting filter: " + string(f))
	}

	query := `
		select
			locations.city_name,
			locations.id,
			weather.at_time,
			weather.%s,
			weather.location_id
		from locations, weather
		where locations.city_name is not null and locations.id = weather.location_id
		order by weather.at_time desc`

	rows, err := GlobalConn.Query(fmt.Sprintf(query, param))
	if err != nil {
		return nil, err
	}

	temps := LocationTemperatureQueryResult{}

	for rows.Next() {
		var (
			cname sql.NullString
			id    sql.NullInt64
			lid   sql.NullInt64
			temp  sql.NullFloat64
		)

		t := time.Time{}

		if err := rows.Scan(&cname, &id, &t, &temp, &lid); err != nil {
			return nil, err
		}

		if !cname.Valid {
			continue
		}

		y, m, d := t.Date()
		mo := int(m)
		city := cname.String

		temps.InitialiseForDate(city, y, mo, d)

		// if _, initialised := temps[city]; !initialised {
		// 	temps[city] = YearlyTemperatureQuery{}
		// }
		// if _, initialised := temps[city][y]; !initialised {
		// 	temps[city][y] = MonthlyTemperatureQuery{}
		// }

		// if _, initialised := temps[city][y][mo]; !initialised {
		// 	temps[city][y][mo] = DailyTemperatureQuery{}
		// }

		// if _, initialised := temps[city][y][mo][d]; !initialised {
		// 	temps[city][y][mo][d] = TemperatureQueries{}
		// }

		if !temp.Valid {
			continue
		}

		// temps[city][y][mo][d] = append(temps[city][y][mo][d], temp.Float64)
		temps.Add(temp.Float64, city, y, mo, d)
	}

	return temps, nil
}

// MonthlyAverageTemperature returns the average temperature for all the months. Currently
// filtering by individual 'months' is not implemented.
func MonthlyAverageTemperature(months ...string) (LocationTemperatureQueryResult, error) {
	query := `
		select
			locations.city_name,
			locations.id,
			weather.at_time,
			weather.temp_low,
			weather.temp_high,
			weather.location_id
		from locations, weather
		where locations.city_name is not null and locations.id = weather.location_id
		order by weather.at_time desc`

	rows, err := GlobalConn.Query(query)
	if err != nil {
		return nil, err
	}

	// temps := monthlyReport{}
	dailyAvgTemps := LocationTemperatureQueryResult{}

	for rows.Next() {
		var (
			cname  sql.NullString
			id     sql.NullInt64
			lid    sql.NullInt64
			templo sql.NullFloat64
			temphi sql.NullFloat64
		)

		t := time.Time{}

		if err := rows.Scan(&cname, &id, &t, &templo, &temphi, &lid); err != nil {
			return nil, err
		}

		if !cname.Valid {
			continue
		}

		y, m, d := t.Date()
		mo := int(m)
		city := cname.String

		dailyAvgTemps.InitialiseForDate(city, y, mo, d)

		// if _, initialised := dailyAvgTemps[city]; !initialised {
		// 	dailyAvgTemps[city] = map[int]map[int]map[int][]float64{}
		// }
		// if _, initialised := dailyAvgTemps[city][y]; !initialised {
		// 	dailyAvgTemps[city][y] = map[int]map[int][]float64{}
		// }
		// if _, initialised := dailyAvgTemps[city][y][mo]; !initialised {
		// 	dailyAvgTemps[city][y][mo] = map[int][]float64{}
		// }
		// if _, initialised := dailyAvgTemps[city][y][mo][d]; !initialised {
		// 	dailyAvgTemps[city][y][mo][d] = []float64{}
		// }

		if !templo.Valid || !temphi.Valid {
			continue
		}

		mid := (templo.Float64 + temphi.Float64) / 2

		// temps[city][y][mo][d] = append(temps[city][y][mo][d], mid)
		dailyAvgTemps.Add(mid, city, y, mo, d)
	}

	monthlyAvgTemps := LocationTemperatureQueryResult{}
	// monthlyAvgTemps := monthlyReport{}

	for c, cities := range dailyAvgTemps {
		for y, year := range cities {
			for m, month := range year {
				var (
					avg             float64
					samplesPerMonth int
					samplesPerDay   int
				)

				for _, day := range month {
					samplesPerMonth++
					for _, temp := range day {
						log.Println(temp)
						samplesPerDay++
						avg += temp
					}
				}

				avg /= float64(samplesPerDay)
				avg /= float64(samplesPerMonth)

				// monthlyAvgTemps[c] = make(map[int]map[int]map[int][]float64)
				// monthlyAvgTemps[c][y] = make(map[int]map[int][]float64)
				// monthlyAvgTemps[c][y][m] = make(map[int][]float64)
				// monthlyAvgTemps[c][y][m][0] = []float64{avg}
				monthlyAvgTemps.InitialiseForDate(c, y, m, 0)
				monthlyAvgTemps.Add(avg, c, y, m, 0)
			}
		}
	}

	return monthlyAvgTemps, nil
}

// AccountRow represents a database row in the 'accounts' table.
type AccountRow struct {
	Name sql.NullString
	ID   sql.NullInt64
}

// NewAccount creates a new row in the database 'accounts' table.
func NewAccount(username string) (*AccountRow, error) {
	query := `
		insert into accounts (user_name)
			values ($1)
			on conflict (user_name)
				do nothing
		returning
			id, user_name`

	rowData := &AccountRow{}
	row := GlobalConn.QueryRow(query, username)

	if err := row.Scan(&rowData.ID, &rowData.Name); err != nil {
		return nil, err
	}

	return rowData, nil
}

// ExistingAccount returns an account with a username matching 'username' from the
// database 'accounts' table.
func ExistingAccount(username string) (*AccountRow, error) {
	query := `select id, user_name from accounts where user_name = $1`

	rowData := &AccountRow{}
	row := GlobalConn.QueryRow(query, username)

	if err := row.Scan(&rowData.ID, &rowData.Name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return rowData, nil
}

// NewBookmarkCollection creates a new row in the database 'bookmarks' table and
// maps it to the account.
func (u *AccountRow) NewBookmarkCollection() (*BookmarkRow, error) {
	query := `
		insert into bookmarks (id, location_ids)
			values ($1, $2)
		returning
			id, location_ids`

	rowData := &BookmarkRow{ID: u.ID}
	row := GlobalConn.QueryRow(query, rowData.ID, rowData.LocationIDs)

	if err := row.Scan(&rowData.ID, &rowData.LocationIDs); err != nil {
		return nil, err
	}

	return rowData, nil
}

// GetBookmarkCollectionIDs gets a row in the database 'bookmarks' table for an account.
func (u *AccountRow) GetBookmarkCollectionIDs() (*BookmarkRow, error) {
	query := `
		select id, location_ids from bookmarks where id = $1`

	rowData := &BookmarkRow{}
	row := GlobalConn.QueryRow(query, u.ID)

	if err := row.Scan(&rowData.ID, &rowData.LocationIDs); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return rowData, nil
}

// UpdateBookmarkCollectionIDs updates a row in the database 'bookmarks' table for the account.
func (u *AccountRow) UpdateBookmarkCollectionIDs(bookmarks ...int) (*BookmarkRow, error) {
	query := `
		update bookmarks
			set location_ids = bookmarks.location_ids || $2
		where
			id = $1
		returning location_ids`

	ids := []int64{}
	for _, b := range bookmarks {
		ids = append(ids, int64(b))
	}

	rowData := &BookmarkRow{ID: u.ID, LocationIDs: pq.Int64Array(ids)}
	row := GlobalConn.QueryRow(query, rowData.ID, rowData.LocationIDs)

	if err := row.Scan(&rowData.LocationIDs); err != nil {
		return nil, err
	}

	return rowData, nil
}

// BookmarkRow represents a database row in the 'bookmarks' table.
type BookmarkRow struct {
	ID          sql.NullInt64
	LocationIDs pq.Int64Array
}

// NamesFromIDs generate a list of location names given the location ids of the 'bookmarks' table row
func (b *BookmarkRow) NamesFromIDs() ([]string, error) {
	query := `select city_name from locations where id = any($1)`

	rows, err := GlobalConn.Query(query, b.LocationIDs)
	if err != nil {
		return nil, err
	}

	names := []string{}

	for rows.Next() {
		var name sql.NullString

		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		if name.Valid {
			names = append(names, name.String)
		}
	}

	return names, nil
}

// IDsFromNames maps location ids from the given location names
func IDsFromNames(names ...string) ([]int, error) {
	query := `select id from locations where city_name = any($1)`

	rows, err := GlobalConn.Query(query, pq.Array(names))
	if err != nil {
		return nil, err
	}

	ids := []int{}

	for rows.Next() {
		var id sql.NullInt64

		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		if id.Valid {
			ids = append(ids, int(id.Int64))
		}
	}

	return ids, nil
}

# readme

* * *

## **setup**

* * *

**with docker**

assumes you have docker and docker-compose installed. also assumes you have an
`openweather` api key.

first:

```
~$ git clone https://github.com/msawangwan/weather.git
~$ cd weather
```

_before spinning up any containers, ensure that you
specify a valid `openweather` api key in the `config/api.env` file -- `API_KEY` **must** have a value set!_

to run the test suite, execute from the project root directory:

```
~$ docker-compose --file docker-compose.test.yml up --build
```

to run the service, execute from the project root directory:

```
~$ docker-compose up --build
```

all configuration is defined from files in the `config/` directory.

* * *

**without docker**

if you don't have docker or don't want to use it, then you will need:

- `golang` with `go` `module` support enabled (*recommended version:* `>=1.12`)
- a `postgres` database (*recommended version:* `>=1.11`)

assuming these requirements are met then ensure these variables are set in the execution environment:

- `API_KEY` (*`openweather` api key*)
- `API_ENDPOINT` (*`openweather` api endpoint*)
- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_HOSTNAME`
- `LISTEN_ADDR`
- `LISTEN_PORT`

(_see the `.env` files in the `config/` directory for examples_)

if all that is in order and the database is running, then:

```
~$ git clone https://github.com/msawangwan/weather.git
~$ cd weather
~$ go run main.go
```

## **endpoints**

**user info**
```
GET /api/v1/account/user
```
*params*
  - `username`

* * *

**register user**
```
POST /api/v1/account/user/register
```

*body*
```
{
    "username": str
}
```

* * *

**get user bookmarks**
```
GET /api/v1/account/user/bookmark
```

*params*
  - `username`

* * *

**update user bookmarks**
```
POST /api/v1/account/user/bookmark
```

*body*
```
{
    "username": str,
    "locations": [
        str,
        ..
    ]
}
```

* * *

**weather for location**
```
GET /api/v1/location/weather
```
*params*
  - `city`

* * *

**weather stats**
```
GET /api/v1/location/weather/stats
```
*params*
  - `count`=`query` (not implemented `labels`)
  - `summary`=`day` (not implemented `mo`|`y`)
  - `temp`=`lows`|`highs`|`avgs`

* * *

## **example**:

*register a new user*
```
~$ curl -d"account.json" -X POST 'localhost:1337/api/v1/account/user/register'
{
    "name": "foobar",
    "id": 1,
    "bookmark_collection_id": 1
}
```
*update an existing users bookmarks*
```
~$ curl -d"@bookmark.json" -X POST 'localhost:1337/api/v1/account/user/bookmark'
{
    "Bookmarks": [
        "London",
        "San Francisco",
        "Budapest"
    ]
}
```
*view user info*
```
~$ curl -X GET 'localhost:1337/api/v1/account/user?username=foobar'
{
    "name": "foobar",
    "id": 1
}
```
*view users bookmarks*
```
~$ curl -X GET 'localhost:1337/api/v1/account/user/bookmark?username=foobar'
{
    "Bookmarks": [
        "London",
        "San Francisco",
        "Budapest"
    ]
}
```
*get weather for a location*
```
~$ curl -X GET 'localhost:1337/api/v1/location/weather?city=London'
{
    "city_name": "London",
    "conditions": [
        "Clear"
    ],
    "low_temp": 279.1499938964844,
    "high_temp": 281.1499938964844,
    "median_temp": 280.1499938964844,
    "at_time": "2019-03-29T21:13:52.22638Z"
}
```
*get stats*
```
$ ~$ curl -X GET 'localhost:1337/api/v1/location/weather/stats?count=query&summary=day'
{
    "count": {
        "location_queries": 6
    },
    "summary": {
        "daily": {
            "Clear": [
                {
                    "CityName": "Budapest",
                    "Date": "2019-03-29T21:13:53.049609Z"
                },
                {
                    "CityName": "Athens",
                    "Date": "2019-03-29T21:13:52.840518Z"
                },
                {
                    "CityName": "London",
                    "Date": "2019-03-29T21:13:52.355404Z"
                },
                {
                    "CityName": "Reno",
                    "Date": "2019-03-29T21:13:52.22638Z"
                }
            ],
            "Haze": [
                {
                    "CityName": "San Francisco",
                    "Date": "2019-03-29T21:13:52.485015Z"
                }
            ],
            "Rain": [
                {
                    "CityName": "New York",
                    "Date": "2019-03-29T21:13:52.617704Z"
                }
            ]
        }
    }
}
```
*get the average temperature by month*
```
~$ curl -X GET 'localhost:1337/api/v1/location/weather/stats?temp=avgs'
{
    "temperatures": {
        "avgs": {
            "Athens": {
                "2019": {
                    "3": {
                        "0": [
                            295.7050018310547
                        ]
                    }
                }
            },
            "Budapest": {
                "2019": {
                    "3": {
                        "0": [
                            281.2050018310547
                        ]
                    }
                }
            },
            "London": {
                "2019": {
                    "3": {
                        "0": [
                            282.1499938964844
                        ]
                    }
                }
            },
            "New York": {
                "2019": {
                    "3": {
                        "0": [
                            286.76499938964844
                        ]
                    }
                }
            },
            "Reno": {
                "2019": {
                    "3": {
                        "0": [
                            280.0950012207031
                        ]
                    }
                }
            },
            "San Francisco": {
                "2019": {
                    "3": {
                        "0": [
                            288.7050018310547
                        ]
                    }
                }
            }
        }
    }
}
```

## **example, cont'd.**

if you want to see most if not all the endpoints in action, there are a few
scripts in the `test/script` directory that make this easy.

  - to populate the database with location and weather data run `populate.sh`.
  - to see most of the queries run `queries.sh`.
  - to see the results of available account related api calls run `user.sh`.

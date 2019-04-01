#!/bin/bash

curl -X GET 'localhost:1337/api/v1/location/weather/stats?temp=avgs'
curl -X GET 'localhost:1337/api/v1/location/weather/stats?count=query&summary=day'

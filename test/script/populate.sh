#!/bin/bash

curl -X GET 'localhost:1337/api/v1/location/weather?city=Reno'
curl -X GET 'localhost:1337/api/v1/location/weather?city=London'
curl -X GET 'localhost:1337/api/v1/location/weather?city=San+Francisco'
curl -X GET 'localhost:1337/api/v1/location/weather?city=New+York'
curl -X GET 'localhost:1337/api/v1/location/weather?city=Athens'
curl -X GET 'localhost:1337/api/v1/location/weather?city=Budapest'

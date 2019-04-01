#!/bin/bash

curl -d"@account.json" -X POST 'localhost:1337/api/v1/account/user/register'
curl -d"@bookmark.json" -X POST 'localhost:1337/api/v1/account/user/bookmark'

curl -X GET 'localhost:1337/api/v1/account/user?username=foobar'
curl -X GET 'localhost:1337/api/v1/account/user/bookmark?username=foobar'

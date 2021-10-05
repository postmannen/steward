#!/bin/bash

# TO USE:
# ./wrapper ./le.crt ./le.key 50222

LE_CRT=$1
LE_KEY=$2
EXPOSED_NATS_PORT=$3

if go run ./steward/scripts/nats-server/generate-nkeys/main.go; then
    echo " * succesfully generated nkeys"
else
    echo " * failed to generate nkeys"
    exit 1
fi

if go run ./steward/scripts/nats-server/create-docker-compose-files/main.go -imageAndVersion=nats-server:2.5.0 -natsConfPath=./nats.conf -leCertPath="$LE_CRT" -leKeyPath="$LE_KEY" -exposedNatsPort="$EXPOSED_NATS_PORT" -templateDir=./steward/scripts/nats-server/create-docker-compose-files/; then
    echo " * succesfully generated docker-compose and env.env file"
else
    echo " * failed to generate docker-compse and env.env file"
    exit 1
fi

if ./steward/scripts/nats-server/generate-nats-conf/generate-nats-conf.sh; then
    echo " * succesfully generated nats.conf"
else
    echo " * failed to generate nats-conf"
    exit 1
fi

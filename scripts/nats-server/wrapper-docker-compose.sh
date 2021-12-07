#!/bin/bash

# TO USE:
# ./wrapper fqdn 50222

FQDN=$1

if docker run -it --rm -v "$PWD:/certs/:rw" -p 80:80 -p 443:443 -e DAEMON=false -e USER_FOLDER=/certs -e PROD=false -e DOMAIN="$FQDN" certupdater:0.1.0; then
    echo " * successfully generated LetsEncrypt certificate"
else
    echo " * failed to generate LetsEncrypt certificate."
    echo " * a docker image of cert updater need to be present on the host."
    echo " * Follow the instructions at https://github.com/RaaLabs/certupdater"
    echo " * to install it as a docker image."
fi

LE_CRT=$PWD/$FQDN/$FQDN.crt
LE_KEY=$PWD/$FQDN/$FQDN.key
EXPOSED_NATS_PORT=$2

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

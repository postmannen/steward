# build stage
FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
RUN git clone https://github.com/RaaLabs/steward.git
WORKDIR /go/steward/cmd
RUN go build -o steward

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /go/steward/cmd/steward /app/

ENV CONFIG_FOLDER "./etc"
ENV SOCKET_FOLDER "./tmp"
ENV DATABASE_FOLDER "./var/lib"
ENV NODE_NAME ""
ENV BROKER_ADDRESS "127.0.0.1:4222"
ENV PROFILING_PORT ""
ENV PROM_HOST_AND_PORT ""
ENV DEFAULT_MESSAGE_TIMEOUT 10
ENV DEFAULT_MESSAGE_RETRIES 3
ENV SUBSCRIBERS_DATA_FOLDER "./var"
ENV CENTRAL_NODE_NAME ""
ENV ROOT_CA_PATH ""
ENV NKEY_SEED_FILE ""

ENV START_PUB_REQ_HELLO 60

ENV START_SUB_REQ_ERROR_LOG ""
ENV START_SUB_REQ_HELLO ""
ENV START_SUB_REQ_TO_FILE_APPEND ""
ENV START_SUB_REQ_TO_FILE ""
ENV START_SUB_REQ_PING ""
ENV START_SUB_REQ_PONG ""
ENV START_SUB_REQ_CLI_COMMAND ""
ENV START_SUB_REQN_CLI_COMMAND ""
ENV START_SUB_REQ_TO_CONSOLE ""
ENV START_SUB_REQ_HTTP_GET ""
ENV START_SUB_REQ_TAIL_FILE ""

CMD ["ash","-c","/app/steward\
    -configFolder=$CONFIG_FOLDER\
    -socketFolder=$SOCKET_FOLDER\
    -databaseFolder=$DATABASE_FOLDER\
    -nodeName=$NODE_NAME\
    -brokerAddress=$BROKER_ADDRESS\
    -profilingPort=$PROFILING_PORT\
    -promHostAndPort=$PROM_HOST_AND_PORT\
    -defaultMessageTimeout=$DEFAULT_MESSAGE_TIMEOUT\
    -defaultMessageRetries=$DEFAULT_MESSAGE_RETRIES\
    -subscribersDataFolder=SUBSCRIBERS_DATA_FOLDER\
    -centralNodeName=$CENTRAL_NODE_NAME\
    -rootCAPath=$ROOT_CA_PATH\
    -nkeySeedFile=$NKEY_SEED_FILE\

    -startPubREQHello=$START_PUB_REQ_HELLO\

    -startSubREQErrorLog=$START_SUB_REQ_ERROR_LOG\
    -startSubREQHello=$START_SUB_REQ_HELLO\
    -startSubREQToFileAppend=$START_SUB_REQ_TO_FILE_APPEND\
    -startSubREQToFile=$START_SUB_REQ_TO_FILE\
    -startSubREQPing=$START_SUB_REQ_PING\
    -startSubREQPong=$START_SUB_REQ_PONG\
    -startSubREQCliCommand=$START_SUB_REQ_CLI_COMMAND\
    -startSubREQnCliCommand=$START_SUB_REQN_CLI_COMMAND\
    -startSubREQToConsole=$START_SUB_REQ_TO_CONSOLE\
    -startSubREQHttpGet=$START_SUB_REQ_HTTP_GET\
    -startSubREQTailFile=$START_SUB_REQ_TAIL_FILE\
    "]
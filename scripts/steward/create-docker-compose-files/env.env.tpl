RING_BUFFER_SIZE=1000
CONFIG_FOLDER=./etc
SOCKET_FOLDER=./tmp
TCP_LISTENER=:8091
HTTP_LISTENER=:8092
DATABASE_FOLDER=./var/lib
NODE_NAME=central
BROKER_ADDRESS={{.BrokerAddressAndPort}}
NATS_CONN_OPT_TIMEOUT=20
NATS_CONNECT_RETRY_INTERVAL=10
NATS_RECONNECT_JITTER=100
NATS_RECONNECT_JITTER_TLS=1
PROFILING_PORT=:6666
PROM_HOST_AND_PORT=:2111
DEFAULT_MESSAGE_TIMEOUT=10
DEFAULT_MESSAGE_RETRIES=3
DEFAULT_METHOD_TIMEOUT=10
SUBSCRIBERS_DATA_FOLDER=./data/
CENTRAL_NODE_NAME=central
ROOT_CA_PATH=""
NKEY_SEED_FILE=/run/secrets/seed
EXPOSE_DATA_FOLDER=:8090
ERROR_MESSAGE_RETRIES=10
ERROR_MESSAGE_TIMEOUT=3
COMPRESSION
SERIALIZATION
SET_BLOCK_PROFILE_RATE=0
ENABLE_SOCKET=1
ENABLE_TUI=0
ENABLE_SIGNATURE_CHECK=0
IS_CENTRAL_AUTH=0
ENABLE_DEBUG=0

START_PUB_REQ_HELLO=60
START_SUB_REQ_ERROR_LOG=true
START_SUB_REQ_HELLO=true
START_SUB_REQ_TO_FILE_APPEND=true
START_SUB_REQ_TO_FILE=true
START_SUB_REQ_TO_FILE_NACK=true
START_SUB_REQ_COPY_FILE_FROM=true
START_SUB_REQ_COPY_FILE_TO=true
START_SUB_REQ_PING=true
START_SUB_REQ_PONG=true
START_SUB_REQ_CLI_COMMAND=true
START_SUB_REQ_TO_CONSOLE=true
START_SUB_REQ_HTTP_GET=true
START_SUB_REQ_HTTP_GET_SCHEDULED=true
START_SUB_REQ_TAIL_FILE=true
START_SUB_REQ_CLI_COMMAND_CONT=true
START_SUB_REQ_RELAY=true
version: "3"

services:
  nats-server:
    build: .
    env_file:
      - env.env
    image: {{.ImageAndVersion}}
    restart: always
    ports:
      - "{{.ExposedNatsPort}}:4222"
    volumes:
      - {{.NatsConfPath}}:/app/nats-server.conf
      - {{.LeCertPath}}:/app/le.crt
      - {{.LeKeyPath}}:/app/le.key
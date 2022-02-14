version: "3"

services:
  nats-server:
    container_name: {{.ContainerName}}
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
    logging:
        driver: "json-file"
        options:
            max-size: "10m"
            max-file: "10"
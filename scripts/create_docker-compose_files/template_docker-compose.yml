version: "3.1"

# docker run -it --rm -v '/home/bt/docker/steward/wireguard:/etc/wireguard' --env-file env.env steward:0.1.6

services:
  steward:
    build: .
    env_file:
      - env.env
    secrets:
      - seed
    image: {{.ImageAndVersion}}
    restart: always
    ports:
      - "{{.ExposedProfilingPort}}:6666"
      - "{{.ExposedPrometheusPort}}:2111"
      - "{{.ExposedDataFolderPort}}:8090"
      - "{{.ExposedTcpListenerPort}}:8091"
      - "{{.ExposedHttpListenerPort}}:8092"
    volumes:
      # - {{.NkeySeedFile}}:/app/seed.txt
      - {{.SocketFolder}}:/app/tmp/:rw

secrets:
  seed:
    file: {{.NkeySeedFile}}
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
    {{if .ExposedProfilingPort}}
      - "{{.ExposedProfilingPort}}:6666"
    {{end}}
    {{if .ExposedPrometheusPort}}
      - "{{.ExposedPrometheusPort}}:2111"
    {{end}}
    {{if .ExposedDataFolderPort}}
      - "{{.ExposedDataFolderPort}}:8090"
    {{end}}
    {{if .ExposedTcpListenerPort}}
      - "{{.ExposedTcpListenerPort}}:8091"
    {{end}}
    {{if .ExposedHttpListenerPort}}
      - "{{.ExposedHttpListenerPort}}:8092"
    {{end}}
    volumes:
      # - {{.NkeySeedFile}}:/app/seed.txt
      - {{.SocketFolder}}:/app/tmp/:rw

secrets:
  seed:
    file: {{.NkeySeedFile}}
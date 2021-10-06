# Create docker compose file for Steward

## docker-compose

 Create a directory where you want your docker compose files, and enter that directory
 
 Clone the steward repository:

```bash
mkdir my_dir
cd my_dir
git clone https://github.com/RaaLabs/steward.git
```

Then create the public and private nkey's by running:

`go run ./steward/scripts/nats-server/generate-nkeys/main.go`

To create the docker-compose and env.env run:

```bash
go run main.go -brokerAddress=127.0.0.1:4223 -nkeySeedFile=./seed.txt
```

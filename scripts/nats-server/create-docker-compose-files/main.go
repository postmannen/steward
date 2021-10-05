// Example for how to run
// go run main.go -imageAndVersion=nats-server:2.5.0 -natsConfPath=./nats.conf -leCertPath=./le.crt -leKeyPath=./le.key -exposedNatsPort=40223

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"text/template"
)

// generateEnv will generate the env.env file.
func generateEnv(templateFile string, eData envData) error {
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("error: parsing template file, check that the templateDir flag is set to the correct path: %v, err: %v", templateFile, err)
	}

	fh, err := os.OpenFile("env.env", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("error: opening env.env for writing: %v, err: %v", templateFile, err)
	}
	defer fh.Close()

	err = tpl.Execute(fh, eData)
	if err != nil {
		return fmt.Errorf("error: exeuting template file: %v, err: %v", templateFile, err)
	}

	return nil
}

// generateDockerCompose will generate the docker-compose.yml file.
func generateDockerCompose(templateFile string, cData composeData) error {
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("error: parsing template file, check that the templateDir flag is set to the correct path: %v, err: %v", templateFile, err)
	}

	fh, err := os.OpenFile("docker-compose.yml", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("error: opening docker-compose.yml for writing: %v, err: %v", templateFile, err)
	}
	defer fh.Close()

	err = tpl.Execute(fh, cData)
	if err != nil {
		return fmt.Errorf("error: exeuting template file: %v, err: %v", templateFile, err)
	}

	return nil
}

type composeData struct {
	ImageAndVersion string
	NatsConfPath    string
	LeCertPath      string
	LeKeyPath       string
	ExposedNatsPort string
}

type envData struct {
	NatsConfig string
	Flags      string
}

func main() {
	imageAndVersion := flag.String("imageAndVersion", "", "The name:version of the docker image to use")
	natsConfPath := flag.String("natsConfPath", "./nats.conf", "the full path of the nats.conf file")
	leCertPath := flag.String("leCertPath", "", "the full path to the LetsEncrypt crt file")
	leKeyPath := flag.String("leKeyPath", "", "the full path to the LetsEncrypt key file")
	exposedNatsPort := flag.String("exposedNatsPort", "", "the port docker will expose nats on")

	flags := flag.String("Flags", "-d", "flags to start nats-server with")

	templateDir := flag.String("templateDir", "./steward/scripts/nats-server/create-docker-compose-files/", "the directory path to where the template files are located")

	flag.Parse()

	cData := composeData{
		ImageAndVersion: *imageAndVersion,
		NatsConfPath:    *natsConfPath,
		LeCertPath:      *leCertPath,
		LeKeyPath:       *leKeyPath,
		ExposedNatsPort: *exposedNatsPort,
	}

	eData := envData{
		Flags: *flags,
	}

	if cData.ImageAndVersion == "" {
		log.Printf("error: -imageAndVersion flag can not be empty\n")
		return
	}
	if cData.NatsConfPath == "" {
		log.Printf("error: -natsConfPath flag can not be empty\n")
		return
	}
	if cData.LeCertPath == "" {
		log.Printf("error: -leCertPath flag can not be empty\n")
		return
	}
	if cData.LeKeyPath == "" {
		log.Printf("error: -leKeyPath flag can not be empty\n")
		return
	}
	if cData.ExposedNatsPort == "" {
		log.Printf("error: -exposedNatsPort flag can not be empty\n")
		return
	}

	{
		p := path.Join(*templateDir, "template_env.env")

		err := generateEnv(p, eData)
		if err != nil {
			log.Printf("%v\n", err)
			return
		}
	}

	{
		p := path.Join(*templateDir, "template_docker-compose.yml")
		err := generateDockerCompose(p, cData)
		if err != nil {
			log.Printf("%v\n", err)
			return
		}
	}
}

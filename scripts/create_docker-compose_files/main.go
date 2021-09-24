package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"text/template"
)

// generateEnv will generate the env.env file.
func generateEnv(templateFile string, brokerAddress string) error {
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("error: parsing template file: %v, err: %v", templateFile, err)
	}

	data := struct {
		BrokerAddressAndPort string
	}{
		BrokerAddressAndPort: brokerAddress,
	}

	fh, err := os.OpenFile("env.env", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("error: opening env.env for writing: %v, err: %v", templateFile, err)
	}
	defer fh.Close()

	err = tpl.Execute(fh, data)
	if err != nil {
		return fmt.Errorf("error: exeuting template file: %v, err: %v", templateFile, err)
	}

	return nil
}

// generateDockerCompose will generate the docker-compose.yml file.
func generateDockerCompose(templateFile string, imageAndVersion string, exposedProfilingPort string, exposedPrometheusPort string, exposedDataFolderPort string, exposedTcpListenerPort string, exposedHttpListenerPort string, nkeySeedFile string, socketFolder string) error {
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("error: parsing template file: %v, err: %v", templateFile, err)
	}

	data := struct {
		ImageAndVersion         string
		ExposedProfilingPort    string
		ExposedPrometheusPort   string
		ExposedDataFolderPort   string
		ExposedTcpListenerPort  string
		ExposedHttpListenerPort string
		NkeySeedFile            string
		SocketFolder            string
	}{
		ImageAndVersion:         imageAndVersion,
		ExposedProfilingPort:    exposedProfilingPort,
		ExposedPrometheusPort:   exposedPrometheusPort,
		ExposedDataFolderPort:   exposedDataFolderPort,
		ExposedTcpListenerPort:  exposedTcpListenerPort,
		ExposedHttpListenerPort: exposedHttpListenerPort,
		NkeySeedFile:            nkeySeedFile,
		SocketFolder:            socketFolder,
	}

	fh, err := os.OpenFile("docker-compose.yml", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("error: opening docker-compose.yml for writing: %v, err: %v", templateFile, err)
	}
	defer fh.Close()

	err = tpl.Execute(fh, data)
	if err != nil {
		return fmt.Errorf("error: exeuting template file: %v, err: %v", templateFile, err)
	}

	return nil
}

func main() {
	brokerAddress := flag.String("brokerAddress", "", "the address:port of the broker to connect to")
	imageAndVersion := flag.String("imageAndVersion", "", "The name:version of the docker image to use")
	exposedProfilingPort := flag.String("exposedProfilingPort", "6666", "the address:port to expose")
	exposedPrometheusPort := flag.String("exposedPrometheusPort", "2111", "the address:port to expose")
	exposedDataFolderPort := flag.String("exposedDataFolderPort", "8090", "the address:port to expose")
	exposedTcpListenerPort := flag.String("exposedTcpListenerPort", "8091", "the address:port to expose")
	exposedHttpListenerPort := flag.String("exposedHttpListenerPort", "8092", "the address:port to expose")
	nkeySeedFile := flag.String("nkeySeedFile", "/tmp/seed.txt", "the complete path of the seed file to mount")
	socketFolder := flag.String("sockerFolder", "/tmp/tmp/", "the complete path of the socket folder to mount")
	flag.Parse()

	if *brokerAddress == "" {
		log.Printf("error: -brokerAddress flag can not be empty\n")
		return
	}

	err := generateEnv("template_env.env", *brokerAddress)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}

	err = generateDockerCompose("template_docker-compose.yml", *imageAndVersion, *exposedProfilingPort, *exposedPrometheusPort, *exposedDataFolderPort, *exposedTcpListenerPort, *exposedHttpListenerPort, *nkeySeedFile, *socketFolder)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
}

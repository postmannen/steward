// Example for how to run
// go run main.go --brokerAddress=localhost:42222

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"
)

func generateNkeys(fileDir string) error {
	cmd := exec.Command("nk", "-gen", "user", "-pubout")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: check if the nk tool is installed on the system: %v", err)
	}

	fmt.Printf("%v\n", string(out))

	scanner := bufio.NewScanner(bytes.NewReader(out))

	for scanner.Scan() {
		text := scanner.Text()
		fmt.Println("scanning line : ", text)

		if strings.HasPrefix(text, "S") {
			p := path.Join(fileDir, "seed.txt")
			err := writekey(p, []byte(text))
			if err != nil {
				return err
			}
		}
		if strings.HasPrefix(text, "U") {
			p := path.Join(fileDir, "user.txt")
			err := writekey(p, []byte(text))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func writekey(fileName string, b []byte) error {
	fh, err := os.OpenFile(fileName, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("error: failed to open create/open file for writing: %v", err)
	}
	defer fh.Close()

	_, err = fh.Write(b)
	if err != nil {
		return fmt.Errorf("error: failed to write file: %v", err)
	}

	return nil
}

// generateEnv will generate the env.env file.
func generateEnv(fileDir string, templateDir string, brokerAddress string) error {
	templateFile := path.Join(templateDir, "env.env.tpl")
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("error: parsing template file, you might need to set the -templateDir path: %v, err: %v", templateFile, err)
	}

	data := struct {
		BrokerAddressAndPort string
	}{
		BrokerAddressAndPort: brokerAddress,
	}

	p := path.Join(fileDir, "env.env")
	fh, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
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
func generateDockerCompose(fileDir string, templateFile string, imageAndVersion string, exposedProfilingPort string, exposedPrometheusPort string, exposedDataFolderPort string, exposedTcpListenerPort string, exposedHttpListenerPort string, nkeySeedFile string, socketFolder string) error {
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("error: parsing template file, you might need to set the -templateDir flag : %v, err: %v", templateFile, err)
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

	p := path.Join(fileDir, "docker-compose.yml")
	fh, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("error: opening docker-compose.yml for writing: %v, err: %v", p, err)
	}
	defer fh.Close()

	err = tpl.Execute(fh, data)
	if err != nil {
		return fmt.Errorf("error: exeuting template file: %v, err: %v", templateFile, err)
	}

	return nil
}

func main() {
	fileDir := flag.String("fileDir", "./", "the directory path for where to store the files")

	brokerAddress := flag.String("brokerAddress", "", "the address:port of the broker to connect to")
	imageAndVersion := flag.String("imageAndVersion", "", "The name:version of the docker image to use")
	exposedProfilingPort := flag.String("exposedProfilingPort", "", "the address:port to expose")
	exposedPrometheusPort := flag.String("exposedPrometheusPort", "", "the address:port to expose")
	exposedDataFolderPort := flag.String("exposedDataFolderPort", "", "the address:port to expose")
	exposedTcpListenerPort := flag.String("exposedTcpListenerPort", "", "the address:port to expose")
	exposedHttpListenerPort := flag.String("exposedHttpListenerPort", "", "the address:port to expose")
	nkeySeedFile := flag.String("nkeySeedFile", "./seed.txt", "the complete path of the seed file to mount")
	socketFolder := flag.String("socketFolder", "./tmp/", "the complete path of the socket folder to mount")

	templateDir := flag.String("templateDir", "./steward/scripts/steward/create-docker-compose-files/", "the directory path to where the template files are located")

	flag.Parse()

	if *brokerAddress == "" {
		log.Printf("error: -brokerAddress flag can not be empty\n")
		return
	}

	{
		err := generateNkeys(*fileDir)
		if err != nil {
			log.Printf("%v\n", err)
			return
		}
	}

	{
		err := generateEnv(*fileDir, *templateDir, *brokerAddress)
		if err != nil {
			log.Printf("%v\n", err)
			return
		}
	}

	{
		template := path.Join(*templateDir, "docker-compose.yml.tpl")
		err := generateDockerCompose(*fileDir, template, *imageAndVersion, *exposedProfilingPort, *exposedPrometheusPort, *exposedDataFolderPort, *exposedTcpListenerPort, *exposedHttpListenerPort, *nkeySeedFile, *socketFolder)
		if err != nil {
			log.Printf("%v\n", err)
			return
		}
	}
}

// Generate both public and private nkeys
// nk -gen user -pubout

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
)

func main() {
	fileDir := flag.String("fileDir", "./", "the directory path for where to store the files")
	flag.Parse()

	cmd := exec.Command("nk", "-gen", "user", "-pubout")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error: check if the nk tool is installed on the system: %v\n", err)
	}

	fmt.Printf("out: %v\n", string(out))

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		text := scanner.Text()

		if strings.HasPrefix(text, "S") {
			p := path.Join(*fileDir, "seed.txt")
			writekey(p, []byte(text))
		}
		if strings.HasPrefix(text, "U") {
			p := path.Join(*fileDir, "user.txt")
			writekey(p, []byte(text))
		}

	}
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

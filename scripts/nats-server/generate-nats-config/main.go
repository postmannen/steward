package main

import (
	"bufio"
	"os"
	"strings"
	"text/template"

	"log"
)

type data struct {
	IP   string
	Name string
	Nkey string
}

const tmpNats string = `
{
	# {{.Name}}
	nkey: {{.Nkey}}
	permissions: {
		publish: {
				allow: ["central.>","errorCentral.>","{{.Name}}.>"]
		}
		subscribe: {
				allow: ["central.>","errorCentral.>","{{.Name}}.>"]
		}
	}
}
`

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		ss := strings.Split(scanner.Text(), ",")
		if len(ss) < 3 {
			continue
		}

		nkey := strings.Split(ss[2], "nkey-user=")

		d := data{
			IP:   ss[0],
			Name: ss[1],
			Nkey: nkey[1],
		}

		tmp, err := template.New("myTemplate").Parse(tmpNats)
		if err != nil {
			log.Printf("error: template parse failed: %v\n", err)
		}

		tmp.Execute(os.Stdout, d)
	}
}

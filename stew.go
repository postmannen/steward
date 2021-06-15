package steward

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"

	"github.com/rivo/tview"
)

type Stew struct {
	stewardSocket string
}

func NewStew() (*Stew, error) {
	stewardSocket := flag.String("stewardSocket", "/usr/local/steward/tmp/steward.sock", "specify the full path of the steward socket file")
	flag.Parse()

	_, err := os.Stat(*stewardSocket)
	if err != nil {
		return nil, fmt.Errorf("error: specify the full path to the steward.sock file: %v", err)
	}

	s := Stew{
		stewardSocket: *stewardSocket,
	}
	return &s, nil
}

func (s *Stew) Start() error {
	err := console()
	if err != nil {
		return fmt.Errorf("error: console failed: %v", err)
	}

	return nil
}

// ---------------------------------------------------

func console() error {
	app := tview.NewApplication()

	reqFillForm := tview.NewForm()
	reqFillForm.SetBorder(true).SetTitle("Request values").SetTitleAlign(tview.AlignLeft)

	// Create a flex layout.
	flexContainer := tview.NewFlex().
		AddItem(reqFillForm, 0, 2, false)

	drawFormREQ(reqFillForm, app)

	if err := app.SetRoot(flexContainer, true).SetFocus(reqFillForm).Run(); err != nil {
		panic(err)
	}

	return nil
}

func drawFormREQ(reqFillForm *tview.Form, app *tview.Application) error {
	m := Message{}

	mRefVal := reflect.ValueOf(m)

	for i := 0; i < mRefVal.NumField(); i++ {
		var err error
		values := []string{"1", "2"}

		switch mRefVal.Type().Field(i).Name {
		case "ToNode":
			// Get nodes from file.
			values, err = getNodeNames("nodeslist.cfg")
			if err != nil {
				return err
			}
			reqFillForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ID":
		case "Data":
			value := `"bash","-c","..."`
			reqFillForm.AddInputField("Data", value, 30, nil, nil)
		case "Method":
			var m Method
			ma := m.GetMethodsAvailable()
			values := []string{}
			for k := range ma.methodhandlers {
				values = append(values, string(k))
			}
			reqFillForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ReplyMethod":
			var m Method
			ma := m.GetMethodsAvailable()
			values := []string{}
			for k := range ma.methodhandlers {
				values = append(values, string(k))
			}
			reqFillForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "FromNode":
		case "ACKTimeout":

			value := 30
			reqFillForm.AddInputField("ACKTimeout", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "Retries":
			value := 1
			reqFillForm.AddInputField("Retries", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyACKTimeout":
			value := 30
			reqFillForm.AddInputField("ACKTimeout", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyRetries":
			value := 1
			reqFillForm.AddInputField("ReplyRetries", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "MethodTimeout":
			value := 120
			reqFillForm.AddInputField("MethodTimeout", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "Directory":
			value := "/some-dir/"
			reqFillForm.AddInputField("Directory", value, 30, nil, nil)
		case "FileExtension":
			value := ".log"
			reqFillForm.AddInputField("FileExtension", value, 30, nil, nil)
		case "Operation":
		case "PreviousMessage":
		case "done":

		default:
			reqFillForm.AddDropDown("no definition "+mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		}

	}

	reqFillForm.
		AddButton("generate file", func() {
			fh, err := os.Create("test.log")
			if err != nil {
				log.Fatalf("error: failed to create test.log file: %v\n", err)
			}
			defer fh.Close()

			for i := 0; i < reqFillForm.GetFormItemCount(); i++ {
				fi := reqFillForm.GetFormItem(i)

				switch v := fi.(type) {
				case *tview.InputField:
					text := v.GetText()
					fmt.Fprintf(fh, "%#v\n\n", text)
				case *tview.DropDown:
					label := v.GetLabel()
					curOpt, text := v.GetCurrentOption()
					fmt.Fprintf(fh, "%v,%v, %v\n\n", label, curOpt, text)
				}

				// fmt.Fprintf(fh, "%+v\n\n", reqFillForm.GetFormItem(i))
			}
		}).
		AddButton("exit", func() {
			app.Stop()
		})

	app.SetFocus(reqFillForm)

	return nil
}

func validateInteger(text string, ch rune) bool {
	if text == "-" {
		return true
	}
	_, err := strconv.Atoi(text)
	return err == nil
}

// getNodes will load all the node names from a file, and return a slice of
// string values, each representing a unique node.
func getNodeNames(filePath string) ([]string, error) {

	fh, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error: unable to open node file: %v", err)
	}
	defer fh.Close()

	nodes := []string{}

	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		node := scanner.Text()
		nodes = append(nodes, node)
	}

	return nodes, nil
}

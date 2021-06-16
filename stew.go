package steward

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"

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

			type msg struct {
				// The node to send the message to
				ToNode node `json:"toNode" yaml:"toNode"`
				// The actual data in the message
				Data []string `json:"data" yaml:"data"`
				// Method, what is this message doing, etc. CLI, syslog, etc.
				Method Method `json:"method" yaml:"method"`
				// ReplyMethod, is the method to use for the reply message.
				// By default the reply method will be set to log to file, but
				// you can override it setting your own here.
				ReplyMethod Method `json:"replyMethod" yaml:"replyMethod"`
				// From what node the message originated
				FromNode node
				// ACKTimeout for waiting for an ack message
				ACKTimeout int `json:"ACKTimeout" yaml:"ACKTimeout"`
				// Resend retries
				Retries int `json:"retries" yaml:"retries"`
				// The ACK timeout of the new message created via a request event.
				ReplyACKTimeout int `json:"replyACKTimeout" yaml:"replyACKTimeout"`
				// The retries of the new message created via a request event.
				ReplyRetries int `json:"replyRetries" yaml:"replyRetries"`
				// Timeout for long a process should be allowed to operate
				MethodTimeout int `json:"methodTimeout" yaml:"methodTimeout"`
				// Directory is a string that can be used to create the
				//directory structure when saving the result of some method.
				// For example "syslog","metrics", or "metrics/mysensor"
				// The type is typically used in the handler of a method.
				Directory string `json:"directory" yaml:"directory"`
				// FileExtension is used to be able to set a wanted extension
				// on a file being saved as the result of data being handled
				// by a method handler.
				FileExtension string `json:"fileExtension" yaml:"fileExtension"`
			}
			m := msg{}
			// Loop trough all the form fields
			for i := 0; i < reqFillForm.GetFormItemCount(); i++ {
				fi := reqFillForm.GetFormItem(i)
				label, value := getLabelAndValue(fi)

				switch label {
				case "ToNode":
					m.ToNode = node(value)
				case "Data":
					sp := strings.Split(value, ",")
					m.Data = sp
				case "Method":
					m.Method = Method(value)
				case "ReplyMethod":
					m.ReplyMethod = Method(value)
				case "ACKTimeout":
					v, _ := strconv.Atoi(value)
					m.ACKTimeout = v
				case "Retries":
					v, _ := strconv.Atoi(value)
					m.Retries = v
				case "ReplyACKTimeout":
					v, _ := strconv.Atoi(value)
					m.ReplyACKTimeout = v
				case "ReplyRetries":
					v, _ := strconv.Atoi(value)
					m.ReplyRetries = v
				case "MethodTimeout":
					v, _ := strconv.Atoi(value)
					m.MethodTimeout = v
				case "Directory":
					m.Directory = value
				case "FileExtension":
					m.FileExtension = value
				}
			}
			msgs := []msg{}
			msgs = append(msgs, m)

			jEnc := json.NewEncoder(fh)
			jEnc.Encode(msgs)
		}).
		AddButton("exit", func() {
			app.Stop()
		})

	app.SetFocus(reqFillForm)

	return nil
}

// Will return the Label And the text Value of a form field.
func getLabelAndValue(fi tview.FormItem) (string, string) {
	var label string
	var value string
	switch v := fi.(type) {
	case *tview.InputField:
		value = v.GetText()
		label = v.GetLabel()
	case *tview.DropDown:
		label = v.GetLabel()
		_, value = v.GetCurrentOption()
	}

	return label, value
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

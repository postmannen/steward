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
	"time"

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
	// Check that the message struct used within stew are up to date, and
	// consistent with the fields used in the main Steward message file.
	// If it throws an error here we need to update the msg struct type,
	// or add a case for the field to except.
	err := checkFieldsMsg()
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	app := tview.NewApplication()

	reqFillForm := tview.NewForm()
	reqFillForm.SetBorder(true).SetTitle("Request values").SetTitleAlign(tview.AlignLeft)

	logView := tview.NewTextView()
	logView.SetBorder(true).SetTitle("Log/Status").SetTitleAlign(tview.AlignLeft)
	logView.SetChangedFunc(func() {
		app.Draw()
	})

	// Create a flex layout.
	flexContainer := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(reqFillForm, 0, 10, false).
		AddItem(logView, 0, 2, false)

	drawFormREQ(reqFillForm, logView, app)

	if err := app.SetRoot(flexContainer, true).SetFocus(reqFillForm).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	return nil
}

// Creating a copy of the real Message struct here to use within the
// field specification, but without the control kind of fields from
// the original to avoid changing them to pointer values in the main
// struct which would be needed when json marshaling to omit those
// empty fields.
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
	// operation are used to give an opCmd and opArg's.
	Operation *Operation `json:"operation,omitempty"`
}

// Will check and compare all the fields of the main message struct
// used in Steward, and the message struct used in Stew that they are
// equal.
// If they are not equal an error will be returned to the user with
// the name of the field that was missing in the Stew message struct.
//
// Some of the fields in the Steward Message struct are used by the
// system for control, and not needed when creating an initial message
// template, and we can add case statements for those fields below
// that we do not wan't to check.
func checkFieldsMsg() error {
	stewardM := Message{}
	stewM := msg{}

	stewardRefVal := reflect.ValueOf(stewardM)
	stewRefVal := reflect.ValueOf(stewM)

	// Loop trough all the fields of the Message struct, and create
	// a an input field or dropdown selector for each field.
	// If a field of the struct is not defined below, it will be
	// created a "no defenition" element, so it we can easily spot
	// Message fields who miss an item in the form.
	for i := 0; i < stewardRefVal.NumField(); i++ {
		found := false

		for ii := 0; ii < stewRefVal.NumField(); ii++ {
			if stewardRefVal.Type().Field(i).Name == stewRefVal.Type().Field(ii).Name {
				found = true
				break
			}
		}

		// Case statements for the fields we don't care about for
		// the message template.
		if !found {
			switch stewardRefVal.Type().Field(i).Name {
			case "ID":
				// Not used in message template.
			case "FromNode":
				// Not used in message template.
			case "PreviousMessage":
				// Not used in message template.
			case "done":
				// Not used in message template.
			default:
				return fmt.Errorf("error: %v within the steward Message struct were not found in the stew msg struct", stewardRefVal.Type().Field(i).Name)

			}
		}
	}

	return nil
}

func drawFormREQ(reqFillForm *tview.Form, logForm *tview.TextView, app *tview.Application) error {
	m := msg{}

	mRefVal := reflect.ValueOf(m)

	// Loop trough all the fields of the Message struct, and create
	// a an input field or dropdown selector for each field.
	// If a field of the struct is not defined below, it will be
	// created a "no defenition" element, so it we can easily spot
	// Message fields who miss an item in the form.
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
			rm := m.getReplyMethods()
			values := []string{}
			for _, k := range rm {
				values = append(values, string(k))
			}
			reqFillForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
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
			values := []string{"todo"}
			reqFillForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)

		default:
			// Add a no definition fields to the form if a a field within the
			// struct were missing an action above, so we can easily detect
			// if there is missing a case action for one of the struct fields.
			reqFillForm.AddDropDown("error: no case for: "+mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		}

	}

	reqFillForm.
		// Add a generate button, which when pressed will loop trouug
		AddButton("generate file", func() {
			fh, err := os.Create("message.json")
			if err != nil {
				log.Fatalf("error: failed to create test.log file: %v\n", err)
			}
			defer fh.Close()

			m := msg{}
			// Loop trough all the form fields
			for i := 0; i < reqFillForm.GetFormItemCount(); i++ {
				fi := reqFillForm.GetFormItem(i)
				label, value := getLabelAndValue(fi)

				switch label {
				case "ToNode":
					m.ToNode = node(value)
				case "Data":
					// Split the comma separated string into a
					// and remove the start and end ampersand.
					sp := strings.Split(value, ",")

					var data []string

					for _, v := range sp {
						// Check if format is correct, return if not.
						pre := strings.HasPrefix(v, "\"")
						suf := strings.HasSuffix(v, "\"")
						if !pre || !suf {
							fmt.Fprintf(logForm, "%v : error: malformed format for command, should be \"cmd\",\"arg1\",\"arg2\" ...\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
							return
						}
						// Remove leading and ending ampersand.
						v = v[1:]
						v = strings.TrimSuffix(v, "\"")

						data = append(data, v)
					}

					m.Data = data
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
				case "Operation":
					m.Operation = nil
				default:
					fmt.Fprintf(logForm, "%v : error: did not find case defenition for how to handle the \"%v\" within the switch statement\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), label)
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

// Will return the Label And the text Value of an input or dropdown form field.
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

// Check if number is int.
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

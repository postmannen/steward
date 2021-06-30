package stew

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/RaaLabs/steward"
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
	pages := tview.NewPages()

	app := tview.NewApplication()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlN {
			pages.SwitchToPage("message")
			return nil
		} else if event.Key() == tcell.KeyCtrlP {
			pages.SwitchToPage("message")
			return nil
		}
		return event
	})

	//info := tview.NewTextView().
	//	SetDynamicColors(true).
	//	SetRegions(true).
	//	SetWrap(false)

	slide := messageSlide(app)

	pages.AddPage("message", slide, true, true)

	// Create the main layout.
	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, true).
		//AddItem(info, 1, 1, false).
		SetBorder(true)

	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	return nil
}

// Check and compare all the fields of the main message struct
// used in Steward, and the message struct used in Stew that they are
// equal.
// If they are not equal an error will be returned to the user with
// the name of the field that was missing in the Stew message struct.
//
// Some of the fields in the Steward Message struct are used by the
// system for control, and not needed when creating an initial message
// template, and we can add case statements for those fields below
// that we do not wan't to check.
func compareMsgAndMessage() error {
	stewardMessage := steward.Message{}
	stewMsg := msg{}

	stewardRefVal := reflect.ValueOf(stewardMessage)
	stewRefVal := reflect.ValueOf(stewMsg)

	// Loop trough all the fields of the Message struct.
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

// -----------------------

// Creating a copy of the real Message struct here to use within the
// field specification, but without the control kind of fields from
// the original to avoid changing them to pointer values in the main
// struct which would be needed when json marshaling to omit those
// empty fields.
type msg struct {
	// The node to send the message to
	ToNode steward.Node `json:"toNode" yaml:"toNode"`
	// The actual data in the message
	Data []string `json:"data" yaml:"data"`
	// Method, what is this message doing, etc. CLI, syslog, etc.
	Method steward.Method `json:"method" yaml:"method"`
	// ReplyMethod, is the method to use for the reply message.
	// By default the reply method will be set to log to file, but
	// you can override it setting your own here.
	ReplyMethod steward.Method `json:"replyMethod" yaml:"replyMethod"`
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
	Operation interface{} `json:"operation,omitempty"`
}

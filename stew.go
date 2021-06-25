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

	"github.com/gdamore/tcell/v2"
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
	c := newConsole()
	err := c.start()
	if err != nil {
		return fmt.Errorf("error: console failed: %v", err)
	}

	return nil
}

// ---------------------------------------------------
type console struct {
	flex          *tview.Flex
	msgInputForm  *tview.Form
	msgOutputForm *tview.TextView
	logForm       *tview.TextView
	app           *tview.Application
}

func newConsole() *console {
	c := console{}
	c.app = tview.NewApplication()

	c.msgInputForm = tview.NewForm()
	c.msgInputForm.SetBorder(true).SetTitle("Request values").SetTitleAlign(tview.AlignLeft)

	c.msgOutputForm = tview.NewTextView()
	c.msgOutputForm.SetBorder(true).SetTitle("Message output").SetTitleAlign(tview.AlignLeft)
	c.msgOutputForm.SetChangedFunc(func() {
		// Will cause the log window to be redrawn as soon as
		// new output are detected.
		c.app.Draw()
	})

	c.logForm = tview.NewTextView()
	c.logForm.SetBorder(true).SetTitle("Log/Status").SetTitleAlign(tview.AlignLeft)
	c.logForm.SetChangedFunc(func() {
		// Will cause the log window to be redrawn as soon as
		// new output are detected.
		c.app.Draw()
	})

	// Create a flex layout.
	//
	// First create the outer flex layout.
	c.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		// Add the top windows with columns.
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(c.msgInputForm, 0, 10, false).
			AddItem(c.msgOutputForm, 0, 10, false),
			0, 10, false).
		// Add the bottom log window.
		AddItem(tview.NewFlex().
			AddItem(c.logForm, 0, 2, false),
			0, 2, false)
	return &c
}

// Will start the console.
func (c *console) start() error {
	// Check that the message struct used within stew are up to date, and
	// consistent with the fields used in the main Steward message file.
	// If it throws an error here we need to update the msg struct type,
	// or add a case for the field to except.
	err := compareMsgAndMessage()
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	c.drawMsgForm()

	if err := c.app.SetRoot(c.flex, true).SetFocus(c.msgInputForm).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	return nil
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
func compareMsgAndMessage() error {
	stewardMessage := Message{}
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

func (c *console) drawMsgForm() error {
	m := msg{}

	// Loop trough all the fields of the Message struct, and create
	// a an input field or dropdown selector for each field.
	// If a field of the struct is not defined below, it will be
	// created a "no defenition" element, so it we can easily spot
	// Message fields who miss an item in the form.
	//
	// INFO: The reason that reflect are being used here is to have
	// a simple way of detecting that we are creating form fields
	// for all the fields in the struct. If we have forgot'en one
	// it will create a "no case" field in the console, to easily
	// detect that a struct field are missing a defenition below.
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

			item := tview.NewDropDown()
			item.SetLabelColor(tcell.ColorIndianRed)
			item.SetLabel(mRefVal.Type().Field(i).Name).SetOptions(values, nil)
			c.msgInputForm.AddFormItem(item)
			//c.msgForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ID":
			// This value is automatically assigned by steward.
		case "Data":
			value := `"bash","-c","..."`
			c.msgInputForm.AddInputField("Data", value, 30, nil, nil)
		case "Method":
			var m Method
			ma := m.GetMethodsAvailable()
			values := []string{}
			for k := range ma.methodhandlers {
				values = append(values, string(k))
			}
			c.msgInputForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ReplyMethod":
			var m Method
			rm := m.getReplyMethods()
			values := []string{}
			for _, k := range rm {
				values = append(values, string(k))
			}
			c.msgInputForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ACKTimeout":
			value := 30
			c.msgInputForm.AddInputField("ACKTimeout", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "Retries":
			value := 1
			c.msgInputForm.AddInputField("Retries", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyACKTimeout":
			value := 30
			c.msgInputForm.AddInputField("ACKTimeout", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyRetries":
			value := 1
			c.msgInputForm.AddInputField("ReplyRetries", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "MethodTimeout":
			value := 120
			c.msgInputForm.AddInputField("MethodTimeout", fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "Directory":
			value := "/some-dir/"
			c.msgInputForm.AddInputField("Directory", value, 30, nil, nil)
		case "FileExtension":
			value := ".log"
			c.msgInputForm.AddInputField("FileExtension", value, 30, nil, nil)
		case "Operation":
			// Prepare the selectedFunc that will be triggered for the operations
			// when a field in the dropdown is selected.
			// This selectedFunc will generate the sub fields of the selected
			// operation, and will also remove any previously drawn operation
			// sub fields from the current form.
			values := []string{"ps", "startProc", "stopProc"}
			selectedFunc := func(option string, optionIndex int) {

				// Prepare the map structure that knows what tview items the
				// operation should contain.
				// Then we can pick from this map later, to know what fields
				// to draw, and also which fields to delete if another operation
				// is selected from the dropdown.

				type formItem struct {
					label    string
					formItem tview.FormItem
				}

				operationFormItems := map[string][]formItem{}

				operationFormItems["ps"] = nil

				operationFormItems["startProc"] = func() []formItem {
					formItems := []formItem{}

					// Get values values to be used for the "Method" dropdown.
					var m Method
					ma := m.GetMethodsAvailable()
					values := []string{}
					for k := range ma.methodhandlers {
						values = append(values, string(k))
					}

					// Create the individual form items, and append them to the
					// formItems slice.
					{
						label := "startProc Method"
						item := tview.NewDropDown()
						item.SetLabel(label).SetOptions(values, nil).SetLabelWidth(25)
						formItems = append(formItems, formItem{label: label, formItem: item})
					}
					{
						label := "startProc AllowedNodes"
						item := tview.NewInputField()
						item.SetLabel(label).SetFieldWidth(30).SetLabelWidth(25)
						formItems = append(formItems, formItem{label: label, formItem: item})
					}

					return formItems
				}()

				operationFormItems["stopProc"] = func() []formItem {
					formItems := []formItem{}

					// RecevingNode node        `json:"receivingNode"`
					// Method       Method      `json:"method"`
					// Kind         processKind `json:"kind"`
					// ID           int         `json:"id"`

					// Get nodes from file.
					values, err = getNodeNames("nodeslist.cfg")
					if err != nil {
						fmt.Fprintf(c.logForm, "%v: error: unable to get nodes\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
						return nil
					}

					{
						label := "stopProc ToNode"
						item := tview.NewDropDown()
						item.SetLabel(label).SetOptions(values, nil).SetLabelWidth(25)
						formItems = append(formItems, formItem{label: label, formItem: item})

					}

					// Get values values to be used for the "Method" dropdown.
					var m Method
					ma := m.GetMethodsAvailable()
					values := []string{}
					for k := range ma.methodhandlers {
						values = append(values, string(k))
					}

					// Create the individual form items, and append them to the
					// formItems slice.
					{
						label := "stopProc Method"
						item := tview.NewDropDown()
						item.SetLabel(label).SetOptions(values, nil).SetLabelWidth(25)
						formItems = append(formItems, formItem{label: label, formItem: item})

					}

					processKind := []string{"publisher", "subscriber"}

					{
						label := "stopProc processKind"
						item := tview.NewDropDown()
						item.SetLabel(label).SetOptions(processKind, nil).SetLabelWidth(25)
						formItems = append(formItems, formItem{label: label, formItem: item})

					}

					{
						label := "stopProc ID"
						item := tview.NewInputField()
						item.SetLabel(label).SetFieldWidth(30).SetLabelWidth(25)
						formItems = append(formItems, formItem{label: label, formItem: item})
					}

					return formItems
				}()

				itemDraw := func(label string) {
					// Delete previously drawn sub operation form items.
					for _, vSlice := range operationFormItems {
						for _, v := range vSlice {
							i := c.msgInputForm.GetFormItemIndex(v.label)
							if i > -1 {
								c.msgInputForm.RemoveFormItem(i)
							}
						}
					}

					// Get and draw the form items to the form.
					formItems := operationFormItems[label]
					for _, v := range formItems {
						c.msgInputForm.AddFormItem(v.formItem)
					}
				}

				switch option {
				case "ps":
					itemDraw("ps")
				case "startProc":
					itemDraw("startProc")
				case "stopProc":
					itemDraw("stopProc")
				default:
					fmt.Fprintf(c.logForm, "%v: error: missing menu item for %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), option)
				}
			}

			c.msgInputForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, selectedFunc).SetItemPadding(1)

		default:
			// Add a no definition fields to the form if a a field within the
			// struct were missing an action above, so we can easily detect
			// if there is missing a case action for one of the struct fields.
			c.msgInputForm.AddDropDown("error: no case for: "+mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		}

	}

	// Add Buttons below the message fields. Like Generate and Exit.
	c.msgInputForm.
		// Add a generate button, which when pressed will loop through all the
		// message form items, and if found fill the value into a msg struct,
		// and at last write it to a file.
		//
		// TODO: Should also add a write directly to socket here.
		AddButton("generate to console", func() {
			// ---
			opCmdStartProc := OpCmdStartProc{}
			opCmdStopProc := OpCmdStopProc{}
			// ---

			// fh, err := os.Create("message.json")
			// if err != nil {
			// 	log.Fatalf("error: failed to create test.log file: %v\n", err)
			// }
			// defer fh.Close()

			c.msgOutputForm.Clear()
			fh := c.msgOutputForm

			m := msg{}
			// Loop trough all the form fields
			for i := 0; i < c.msgInputForm.GetFormItemCount(); i++ {
				fi := c.msgInputForm.GetFormItem(i)
				label, value := getLabelAndValue(fi)

				switch label {
				case "ToNode":
					if value == "" {
						fmt.Fprintf(c.logForm, "%v : error: missing ToNode \n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
						return
					}

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
							fmt.Fprintf(c.logForm, "%v : error: missing or malformed format for command, should be \"cmd\",\"arg1\",\"arg2\" ...\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
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
					// We need to check what type of operation it is, and pick
					// the correct struct type, and fill it with values
					switch value {
					case "ps":
						//TODO
					case "startProc":
						m.Operation = &opCmdStartProc
					case "stopProc":
						m.Operation = &opCmdStopProc
					default:
						m.Operation = nil
					}
				case "startProc Method":
					if value == "" {
						fmt.Fprintf(c.logForm, "%v : error: missing startProc Method\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
						return
					}
					opCmdStartProc.Method = Method(value)
				case "startProc AllowedNodes":
					// Split the comma separated string into a
					// and remove the start and end ampersand.
					sp := strings.Split(value, ",")

					var allowedNodes []node

					for _, v := range sp {
						// Check if format is correct, return if not.
						pre := strings.HasPrefix(v, "\"") || !strings.HasPrefix(v, ",") || !strings.HasPrefix(v, "\",") || !strings.HasPrefix(v, ",\"")
						suf := strings.HasSuffix(v, "\"")
						if !pre || !suf {
							fmt.Fprintf(c.logForm, "%v : error: malformed format for command, should be \"cmd\",\"arg1\",\"arg2\" ...\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
							return
						}
						// Remove leading and ending ampersand.
						v = v[1:]
						v = strings.TrimSuffix(v, "\"")

						allowedNodes = append(allowedNodes, node(v))
					}

					opCmdStartProc.AllowedNodes = allowedNodes
				default:
					fmt.Fprintf(c.logForm, "%v : error: did not find case defenition for how to handle the \"%v\" within the switch statement\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), label)
				}
			}
			msgs := []msg{}
			msgs = append(msgs, m)

			msgsIndented, err := json.MarshalIndent(msgs, "", "    ")
			if err != nil {
				fmt.Fprintf(c.logForm, "%v : error: jsonIndent failed: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
			}

			_, err = fh.Write(msgsIndented)
			if err != nil {
				fmt.Fprintf(c.logForm, "%v : error: write to fh failed: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
			}
		}).

		// Add exit button.
		AddButton("exit", func() {
			c.app.Stop()
		})

	c.app.SetFocus(c.msgInputForm)

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
	Operation interface{} `json:"operation,omitempty"`
}

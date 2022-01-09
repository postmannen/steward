package steward

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type tui struct {
}

func newTui() (*tui, error) {
	s := tui{}
	return &s, nil
}

type slide struct {
	name      string
	key       tcell.Key
	primitive tview.Primitive
}

func (s *tui) Start() error {
	pages := tview.NewPages()

	app := tview.NewApplication()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyF1 {
			pages.SwitchToPage("info")
			return nil
		} else if event.Key() == tcell.KeyF2 {
			pages.SwitchToPage("message")
			return nil
		}
		return event
	})

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)

	// The slides to draw, and their name.
	// NB: This slice is being looped over further below, to create the menu
	// elements. If adding a new slide, make sure that slides are ordered in
	// chronological order, so we can auto generate the info menu with it's
	// corresponding F key based on the slice index+1.
	slides := []slide{
		{name: "message", key: tcell.KeyF2, primitive: messageSlide(app)},
		{name: "info", key: tcell.KeyF1, primitive: infoSlide(app)},
	}

	// Add on page for each slide.
	for i, v := range slides {
		if i == 0 {
			pages.AddPage(v.name, v.primitive, true, true)
			fmt.Fprintf(info, " F%v:%v  ", i+1, v.name)
			continue
		}

		pages.AddPage(v.name, v.primitive, true, false)
		fmt.Fprintf(info, " F%v:%v  ", i+1, v.name)
	}

	// Create the main layout.
	layout := tview.NewFlex()
	//layout.SetBorder(true)
	layout.SetDirection(tview.FlexRow).
		AddItem(pages, 0, 10, true).
		AddItem(info, 1, 1, false)

	root := app.SetRoot(layout, true)
	root.EnableMouse(true)

	if err := root.Run(); err != nil {
		log.Printf("error: root.Run(): %v\n", err)
		os.Exit(1)
	}

	return nil
}

func infoSlide(app *tview.Application) tview.Primitive {
	flex := tview.NewFlex()
	flex.SetTitle("info")
	flex.SetBorder(true)

	textView := tview.NewTextView()
	flex.AddItem(textView, 0, 1, false)

	fmt.Fprintf(textView, "Information page for Stew.\n")

	return flex
}

func messageSlide(app *tview.Application) tview.Primitive {
	type pageMessage struct {
		flex          *tview.Flex
		msgInputForm  *tview.Form
		msgOutputForm *tview.TextView
		logForm       *tview.TextView
	}

	p := pageMessage{}
	app = tview.NewApplication()

	p.msgInputForm = tview.NewForm()
	p.msgInputForm.SetBorder(true).SetTitle("Request values").SetTitleAlign(tview.AlignLeft)

	p.msgOutputForm = tview.NewTextView()
	p.msgOutputForm.SetBorder(true).SetTitle("Message output").SetTitleAlign(tview.AlignLeft)
	p.msgOutputForm.SetChangedFunc(func() {
		// Will cause the log window to be redrawn as soon as
		// new output are detected.
		app.Draw()
	})

	p.logForm = tview.NewTextView()
	p.logForm.SetBorder(true).SetTitle("Log/Status").SetTitleAlign(tview.AlignLeft)
	p.logForm.SetChangedFunc(func() {
		// Will cause the log window to be redrawn as soon as
		// new output are detected.
		app.Draw()
	})

	// Create a flex layout.
	//
	// First create the outer flex layout.
	p.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		// Add the top windows with columns.
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(p.msgInputForm, 0, 10, false).
			AddItem(p.msgOutputForm, 0, 10, false),
			0, 10, false).
		// Add the bottom log window.
		AddItem(tview.NewFlex().
			AddItem(p.logForm, 0, 2, false),
			0, 2, false)

	m := Message{}

	// Draw all the message input field with values on the screen.
	//
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

		fieldName := mRefVal.Type().Field(i).Name

		switch fieldName {
		case "_":
		case "ToNode":
			// Get nodes from file.
			values, err := getNodeNames("nodeslist.cfg")
			if err != nil {
				log.Printf("error: unable to open file: %v\n", err)
				os.Exit(1)
			}

			item := tview.NewDropDown()
			item.SetLabelColor(tcell.ColorIndianRed)
			item.SetLabel(fieldName).SetOptions(values, nil)
			p.msgInputForm.AddFormItem(item)
			//c.msgForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ToNodes":
			value := `"ship1","ship2","ship3"`
			p.msgInputForm.AddInputField(fieldName, value, 30, nil, nil)
		case "ID":
		case "Data":
		case "Method":
			var m Method
			ma := m.GetMethodsAvailable()
			values := []string{}
			for k := range ma.Methodhandlers {
				values = append(values, string(k))
			}
			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
		case "MethodArgs":
			value := ``
			p.msgInputForm.AddInputField(fieldName, value, 30, nil, nil)
		case "ReplyMethod":
			var m Method
			rm := m.GetReplyMethods()
			values := []string{}
			for _, k := range rm {
				values = append(values, string(k))
			}
			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
		case "ReplyMethodArgs":
			value := ``
			p.msgInputForm.AddInputField(fieldName, value, 30, nil, nil)
		case "IsReply":
		case "FromNode":
		case "ACKTimeout":
			value := 30
			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "Retries":
			value := 1
			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyACKTimeout":
			value := 30
			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyRetries":
			value := 1
			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "MethodTimeout":
			value := 120
			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "ReplyMethodTimeout":
			value := 120
			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), 30, validateInteger, nil)
		case "Directory":
			value := "/some-dir/"
			p.msgInputForm.AddInputField(fieldName, value, 30, nil, nil)
		case "FileName":
			value := ".log"
			p.msgInputForm.AddInputField(fieldName, value, 30, nil, nil)
		case "PreviousMessage":
		case "RelayViaNode":
			// Get nodes from file.
			values, err := getNodeNames("nodeslist.cfg")
			if err != nil {
				log.Printf("error: unable to open file: %v\n", err)
				os.Exit(1)
			}

			item := tview.NewDropDown()
			item.SetLabelColor(tcell.ColorIndianRed)
			item.SetLabel(fieldName).SetOptions(values, nil)
			p.msgInputForm.AddFormItem(item)
			//c.msgForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "RelayReplyMethod":
			var m Method
			rm := m.GetReplyMethods()
			values := []string{}
			for _, k := range rm {
				values = append(values, string(k))
			}
			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
		case "RelayOriginalViaNode":
		case "RelayFromNode":
		case "RelayToNode":
		case "RelayOriginalMethod":
		case "done":

		default:
			// Add a no definition fields to the form if a a field within the
			// struct were missing an action above, so we can easily detect
			// if there is missing a case action for one of the struct fields.

			p.msgInputForm.AddDropDown("error: no case for: "+fieldName, []string{"1", "2"}, 0, nil).SetItemPadding(1)
		}

	}

	// Add Buttons below the message fields. Like Generate and Exit.
	p.msgInputForm.
		// Add a generate button, which when pressed will loop through all the
		// message form items, and if found fill the value into a msg struct,
		// and at last write it to a file.
		//
		// TODO: Should also add a write directly to socket here.
		AddButton("generate to console", func() {
			// fh, err := os.Create("message.json")
			// if err != nil {
			// 	log.Fatalf("error: failed to create test.log file: %v\n", err)
			// }
			// defer fh.Close()

			p.msgOutputForm.Clear()
			fh := p.msgOutputForm

			m := Message{}
			// Loop trough all the form fields, check the value of each
			// form field, and add the value to m.
			for i := 0; i < p.msgInputForm.GetFormItemCount(); i++ {
				fi := p.msgInputForm.GetFormItem(i)
				label, value := getLabelAndValue(fi)

				switch label {
				case "ToNode":
					if value == "" {
						fmt.Fprintf(p.logForm, "%v : error: missing ToNode \n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
						return
					}

					m.ToNode = Node(value)
				case "ToNodes":
					// Split the comma separated string into a
					// and remove the start and end ampersand.
					sp := strings.Split(value, ",")

					var toNodes []Node

					for _, v := range sp {
						// Check if format is correct, return if not.
						pre := strings.HasPrefix(v, "\"")
						suf := strings.HasSuffix(v, "\"")
						if !pre || !suf {
							fmt.Fprintf(p.logForm, "%v : error: missing or malformed format for command, should be \"cmd\",\"arg1\",\"arg2\" ...\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
							return
						}
						// Remove leading and ending ampersand.
						v = v[1:]
						v = strings.TrimSuffix(v, "\"")

						toNodes = append(toNodes, Node(v))
					}

					m.ToNodes = toNodes
				case "ID":
				case "Data":
				case "Method":
					m.Method = Method(value)
				case "MethodArgs":
					// Split the comma separated string into a
					// and remove the start and end ampersand.

					methodArgs := []string{}

					if value != "" {

						sp := strings.Split(value, ",")
						for _, v := range sp {
							// Check if format is correct, return if not.
							pre := strings.HasPrefix(v, "\"")
							suf := strings.HasSuffix(v, "\"")
							if !pre || !suf {
								fmt.Fprintf(p.logForm, "%v : error: missing or malformed format for command, should be \"cmd\",\"arg1\",\"arg2\" ...\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
								return
							}
							// Remove leading and ending ampersand.
							v = v[1:]
							v = strings.TrimSuffix(v, "\"")

							methodArgs = append(methodArgs, v)
						}
					}

					m.MethodArgs = methodArgs
				case "ReplyMethod":
					m.ReplyMethod = Method(value)
				case "ReplyMethodArgs":
					// Split the comma separated string into a
					// and remove the start and end ampersand.
					var methodArgs []string

					if value != "" {
						sp := strings.Split(value, ",")

						for _, v := range sp {
							// Check if format is correct, return if not.
							pre := strings.HasPrefix(v, "\"")
							suf := strings.HasSuffix(v, "\"")
							if !pre || !suf {
								fmt.Fprintf(p.logForm, "%v : error: missing or malformed format for command, should be \"cmd\",\"arg1\",\"arg2\" ...\n", time.Now().Format("Mon Jan _2 15:04:05 2006"))
								return
							}
							// Remove leading and ending ampersand.
							v = v[1:]
							v = strings.TrimSuffix(v, "\"")

							methodArgs = append(methodArgs, v)
						}
					}

					m.ReplyMethodArgs = methodArgs
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
				case "ReplyMethodTimeout":
					v, _ := strconv.Atoi(value)
					m.ReplyMethodTimeout = v
				case "Directory":
					m.Directory = value
				case "FileName":
					m.FileName = value
				case "RelayViaNode":
					m.RelayViaNode = Node(value)
				case "RelayReplyMethod":
					m.RelayReplyMethod = Method(value)

				default:
					fmt.Fprintf(p.logForm, "%v : error: did not find case definition for how to handle the \"%v\" within the switch statement\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), label)
				}
			}
			msgs := []Message{}
			msgs = append(msgs, m)

			msgsIndented, err := json.MarshalIndent(msgs, "", "    ")
			if err != nil {
				fmt.Fprintf(p.logForm, "%v : error: jsonIndent failed: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
			}

			_, err = fh.Write(msgsIndented)
			if err != nil {
				fmt.Fprintf(p.logForm, "%v : error: write to fh failed: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
			}
		}).

		// Add exit button.
		AddButton("exit", func() {
			app.Stop()
		})

	app.SetFocus(p.msgInputForm)

	return p.flex
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
func getNodeNames(fileName string) ([]string, error) {

	dirPath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error: tui: unable to get working directory: %v", err)
	}

	filePath := filepath.Join(dirPath, fileName)
	log.Printf(" * filepath : %v\n", filePath)

	fh, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error: tui: you should create a file named nodeslist.cfg with all your nodes : %v", err)
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

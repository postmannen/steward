package steward

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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

// ---------------------------------------------------------------------
// Main structure
// ---------------------------------------------------------------------

// tui holds general types used within the tui.
type tui struct {
	toConsoleCh    chan []string
	toRingbufferCh chan []subjectAndMessage
	ctx            context.Context
	nodeName       Node
}

// newTui returns a new tui.
func newTui(nodeName Node) (*tui, error) {
	ch := make(chan []string)
	s := tui{
		toConsoleCh: ch,
		nodeName:    nodeName,
	}
	return &s, nil
}

// slide holds the information about a slide
type slide struct {
	name      string
	key       tcell.Key
	primitive tview.Primitive
}

// Start will start the tui.
func (t *tui) Start(ctx context.Context, toRingBufferCh chan []subjectAndMessage) error {
	t.ctx = ctx
	t.toRingbufferCh = toRingBufferCh

	pages := tview.NewPages()

	app := tview.NewApplication()

	// Check if F key is pressed, and switch slide accordingly.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyF1:
			pages.SwitchToPage("console")
			return nil
		case tcell.KeyF2:
			pages.SwitchToPage("message")
			return nil
		case tcell.KeyF3:
			pages.SwitchToPage("info")
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
		{name: "console", key: tcell.KeyF1, primitive: t.console(app)},
		{name: "message", key: tcell.KeyF2, primitive: t.messageSlide(app)},
		{name: "info", key: tcell.KeyF3, primitive: t.infoSlide(app)},
	}

	// Add a page for each slide.
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

// ---------------------------------------------------------------------
// Slides
// ---------------------------------------------------------------------

func (t *tui) infoSlide(app *tview.Application) tview.Primitive {
	flex := tview.NewFlex()
	flex.SetTitle("info")
	flex.SetBorder(true)

	textView := tview.NewTextView()
	flex.AddItem(textView, 0, 1, false)

	fmt.Fprintf(textView, "Information page for Stew.\n")

	return flex
}

// drawMessageInputFields
//
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
func drawMessageInputFields(p pageMessage, m tuiMessage) {
	fieldWidth := 0

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
			}

			if m.ToNode != nil && *m.ToNode != "" {
				tmp := []string{string(*m.ToNode)}
				tmp = append(tmp, values...)
				values = tmp
			}

			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
			//c.msgForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "ToNodes":
			if m.ToNodes == nil {
				p.msgInputForm.AddInputField(fieldName, "", fieldWidth, nil, nil)
				continue
			}

			if len(*m.ToNodes) != 0 {
				val1 := *m.ToNodes
				var val2 string
				for i, v := range val1 {
					if i == 0 {
						val2 = fmt.Sprintf("\"%v\"", v)
						continue
					}

					val2 = fmt.Sprintf("%v,\"%v\"", val2, v)
				}

				p.msgInputForm.AddInputField(fieldName, val2, fieldWidth, nil, nil)
			}

		case "Method":
			var v Method
			ma := v.GetMethodsAvailable()
			values := []string{}
			for k := range ma.Methodhandlers {
				values = append(values, string(k))
			}

			if m.Method != nil && *m.Method != "" {
				tmp := []string{string(*m.Method)}
				tmp = append(tmp, values...)
				values = tmp
			}

			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
		case "MethodArgs":
			if m.MethodArgs == nil {
				p.msgInputForm.AddInputField(fieldName, "", 0, nil, nil)
				continue
			}

			if len(*m.MethodArgs) != 0 {
				val1 := *m.MethodArgs
				var val2 string
				for i, v := range val1 {
					if i == 0 {
						val2 = fmt.Sprintf("\"%v\"", v)
						continue
					}

					val2 = fmt.Sprintf("%v,\"%v\"", val2, v)
				}

				p.msgInputForm.AddInputField(fieldName, val2, 0, nil, nil)
			}

		case "ReplyMethod":
			var v Method
			rm := v.GetReplyMethods()
			values := []string{}
			for _, k := range rm {
				values = append(values, string(k))
			}

			if m.ReplyMethod != nil && *m.ReplyMethod != "" {
				tmp := []string{string(*m.ReplyMethod)}
				tmp = append(tmp, values...)
				values = tmp
			}

			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
		case "ReplyMethodArgs":
			if m.ReplyMethodArgs == nil {
				p.msgInputForm.AddInputField(fieldName, "", fieldWidth, nil, nil)
				continue
			}

			if len(*m.ReplyMethodArgs) != 0 {
				val1 := *m.ReplyMethodArgs
				var val2 string
				for i, v := range val1 {
					if i == 0 {
						val2 = fmt.Sprintf("\"%v\"", v)
						continue
					}

					val2 = fmt.Sprintf("%v,\"%v\"", val2, v)
				}

				p.msgInputForm.AddInputField(fieldName, val2, fieldWidth, nil, nil)
			}
		case "ACKTimeout":
			value := 30

			if m.ACKTimeout != nil && *m.ACKTimeout != 0 {
				value = *m.ACKTimeout
			}

			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), fieldWidth, validateInteger, nil)
		case "Retries":
			value := 1

			if m.Retries != nil && *m.Retries != 0 {
				value = *m.Retries
			}

			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), fieldWidth, validateInteger, nil)
		case "ReplyACKTimeout":
			value := 30

			if m.ReplyACKTimeout != nil && *m.ReplyACKTimeout != 0 {
				value = *m.ReplyACKTimeout
			}

			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), fieldWidth, validateInteger, nil)
		case "ReplyRetries":
			value := 1

			if m.ReplyRetries != nil && *m.ReplyRetries != 0 {
				value = *m.ReplyRetries
			}

			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), fieldWidth, validateInteger, nil)
		case "MethodTimeout":
			value := 120

			if m.MethodTimeout != nil && *m.MethodTimeout != 0 {
				value = *m.MethodTimeout
			}

			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), fieldWidth, validateInteger, nil)
		case "ReplyMethodTimeout":
			value := 120

			if m.ReplyMethodTimeout != nil && *m.ReplyMethodTimeout != 0 {
				value = *m.ReplyMethodTimeout
			}

			p.msgInputForm.AddInputField(fieldName, fmt.Sprintf("%d", value), fieldWidth, validateInteger, nil)
		case "Directory":
			value := "/some-dir/"

			if m.Directory != nil && *m.Directory != "" {
				value = *m.Directory
			}

			p.msgInputForm.AddInputField(fieldName, value, fieldWidth, nil, nil)
		case "FileName":
			value := ".log"

			if m.FileName != nil && *m.FileName != "" {
				value = *m.FileName
			}

			p.msgInputForm.AddInputField(fieldName, value, fieldWidth, nil, nil)
		case "RelayViaNode":
			// Get nodes from file.
			values, err := getNodeNames("nodeslist.cfg")
			if err != nil {
				log.Printf("error: unable to open file: %v\n", err)
				os.Exit(1)
			}

			if m.RelayViaNode != nil && *m.RelayViaNode != "" {
				tmp := []string{string(*m.RelayViaNode)}
				tmp = append(tmp, values...)
				values = tmp
			}

			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
			//c.msgForm.AddDropDown(mRefVal.Type().Field(i).Name, values, 0, nil).SetItemPadding(1)
		case "RelayReplyMethod":
			var v Method
			rm := v.GetReplyMethods()
			values := []string{}
			for _, k := range rm {
				values = append(values, string(k))
			}

			if m.RelayReplyMethod != nil && *m.RelayReplyMethod != "" {
				tmp := []string{string(*m.RelayReplyMethod)}
				tmp = append(tmp, values...)
				values = tmp
			}

			p.msgInputForm.AddDropDown(fieldName, values, 0, nil).SetItemPadding(1)
		default:
			// Add a no definition fields to the form if a a field within the
			// struct were missing an action above, so we can easily detect
			// if there is missing a case action for one of the struct fields.

			p.msgInputForm.AddDropDown("error: no case for: "+fieldName, []string{"1", "2"}, 0, nil).SetItemPadding(1)
		}

	}

}

// pageMessage is a struct for holding all the main forms and
// views used in the message slide, so we can easily reference
// them later in the code.
type pageMessage struct {
	flex          *tview.Flex
	msgInputForm  *tview.Form
	msgOutputForm *tview.TextView
	logForm       *tview.TextView
	saveForm      *tview.Form
}

// messageSlide is the main function for setting up the slides.
func (t *tui) messageSlide(app *tview.Application) tview.Primitive {

	p := pageMessage{}

	p.msgInputForm = tview.NewForm()
	p.msgInputForm.SetBorder(true).SetTitle("Message input").SetTitleAlign(tview.AlignLeft)

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

	p.saveForm = tview.NewForm()
	p.saveForm.SetBorder(true).SetTitle("Save message").SetTitleAlign(tview.AlignLeft)

	// Create a flex layout.
	//
	// Create the outer flex layout.
	p.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		// Add a flex for the top windows with columns.
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(p.msgInputForm, 0, 10, false).
			// Add a new flex for splitting output form horizontally.
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				// Add the message output form.
				AddItem(p.msgOutputForm, 0, 10, false).
				// Add the save message form.
				AddItem(p.saveForm, 0, 2, false),
				0, 10, false),
			0, 10, false).
		// Add a flex for the bottom log window.
		AddItem(tview.NewFlex().
			// Add the log form.
			AddItem(p.logForm, 0, 2, false),
			0, 1, false)

	m := tuiMessage{}

	// ---

	// Add a dropdown menu to select message files to use.

	msgsValues := getMessageNames(p.logForm)

	msgDropdown := tview.NewDropDown()
	msgDropdown.SetLabelColor(tcell.ColorIndianRed)
	msgDropdown.SetLabel("message").SetOptions(msgsValues, func(msgFileName string, index int) {
		filePath := filepath.Join("messages", msgFileName)
		fh, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(p.logForm, "error: failed to open message file: %v\n", err)
			return
		}
		defer fh.Close()

		fileContent, err := io.ReadAll(fh)
		if err != nil {
			fmt.Fprintf(p.logForm, "error: failed to read message file: %v\n", err)
			return
		}

		fmt.Fprintf(p.logForm, " * DEBUG: %v\n", string(fileContent))

		var msgs []tuiMessage

		err = json.Unmarshal(fileContent, &msgs)
		if err != nil {
			fmt.Fprintf(p.logForm, "error: json unmarshal of file content failed: %v\n", err)
			return
		}

		m = msgs[0]
		fmt.Fprintf(p.logForm, " * DEBUG: %v\n", m.MethodArgs)

		// Clear the form.
		p.msgInputForm.Clear(false)
		// Add a self dropdown when selected since it is not a
		// part of drawing the input fields function.
		p.msgInputForm.AddFormItem(msgDropdown)
		drawMessageInputFields(p, m)
	})
	p.msgInputForm.AddFormItem(msgDropdown)

	// ---

	// Draw all the message input field with values on the screen.
	drawMessageInputFields(p, m)

	// Variable to hold the last output created when the generate button have
	// been pushed.
	var lastGeneratedMessage []byte
	var saveFileName string

	// Add Buttons below the message fields. Like Generate and Exit.
	p.msgInputForm.
		// Add a generate button, which when pressed will loop through all the
		// message form items, and if found fill the value into a msg struct,
		// and at last write it to a file.
		AddButton("generate to console", func() {
			p.msgOutputForm.Clear()

			m := tuiMessage{}
			// Loop trough all the form fields, check the value of each
			// form field, and add the value to m.
			for i := 0; i < p.msgInputForm.GetFormItemCount(); i++ {
				fi := p.msgInputForm.GetFormItem(i)
				label, value := getLabelAndValue(fi)

				switch label {
				case "message":
				case "ToNode":
					v := Node(value)
					m.ToNode = &v
				case "ToNodes":
					slice, err := stringToNode(value)
					if err != nil {
						fmt.Fprintf(p.logForm, "%v : error: ToNodes missing or malformed format, should be \"arg0\",\"arg1\",\"arg2\", %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
						return
					}

					m.ToNodes = slice
				case "Method":
					v := Method(value)
					m.Method = &v
				case "MethodArgs":
					slice, err := stringToSlice(value)
					if err != nil {
						fmt.Fprintf(p.logForm, "%v : error: MethodArgs missing or malformed format, should be \"arg0\",\"arg1\",\"arg2\", %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
						return
					}

					m.MethodArgs = slice
				case "ReplyMethod":
					v := Method(value)
					m.ReplyMethod = &v
				case "ReplyMethodArgs":
					slice, err := stringToSlice(value)
					if err != nil {
						fmt.Fprintf(p.logForm, "%v : error: ReplyMethodArgs missing or malformed format, should be \"arg0\",\"arg1\",\"arg2\", %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
						return
					}

					m.ReplyMethodArgs = slice
				case "ACKTimeout":
					v, _ := strconv.Atoi(value)
					m.ACKTimeout = &v
				case "Retries":
					v, _ := strconv.Atoi(value)
					m.Retries = &v
				case "ReplyACKTimeout":
					v, _ := strconv.Atoi(value)
					m.ReplyACKTimeout = &v
				case "ReplyRetries":
					v, _ := strconv.Atoi(value)
					m.ReplyRetries = &v
				case "MethodTimeout":
					v, _ := strconv.Atoi(value)
					m.MethodTimeout = &v
				case "ReplyMethodTimeout":
					v, _ := strconv.Atoi(value)
					m.ReplyMethodTimeout = &v
				case "Directory":
					m.Directory = &value
				case "FileName":
					m.FileName = &value
				case "RelayViaNode":
					v := Node(value)
					m.RelayViaNode = &v
				case "RelayReplyMethod":
					v := Method(value)
					m.RelayReplyMethod = &v

				default:
					fmt.Fprintf(p.logForm, "%v : error: did not find case definition for how to handle the \"%v\" within the switch statement\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), label)
					return
				}
			}
			msgs := []tuiMessage{}
			msgs = append(msgs, m)

			// msgsIndented, err := json.MarshalIndent(msgs, "", "    ")
			buf := new(bytes.Buffer)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "    ")
			err := enc.Encode(msgs)
			if err != nil {
				fmt.Fprintf(p.logForm, "%v : error: jsonIndent failed: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
			}

			msgsIndented := buf.Bytes()

			// Copy the message to a variable outside this scope so we can use
			// the content for example if we want to save the message to file.
			lastGeneratedMessage = msgsIndented

			_, err = p.msgOutputForm.Write(msgsIndented)
			if err != nil {
				fmt.Fprintf(p.logForm, "%v : error: write to fh failed: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), err)
			}
		}).

		// Add exit button.
		AddButton("exit", func() {
			app.Stop()
		})

	app.SetFocus(p.msgInputForm)

	p.saveForm.
		AddInputField("FileName", "", 40, nil, func(text string) {
			saveFileName = text
		}).
		AddButton("save", func() {
			messageFolder := "messages"

			if saveFileName == "" {
				fmt.Fprintf(p.logForm, "error: missing filename\n")
				return
			}

			if _, err := os.Stat(messageFolder); os.IsNotExist(err) {
				err := os.MkdirAll(messageFolder, 0700)
				if err != nil {
					fmt.Fprintf(p.logForm, "error: failed to create messages folder: %v\n", err)
					return
				}
			}

			file := filepath.Join(messageFolder, saveFileName)
			fh, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				fmt.Fprintf(p.logForm, "error: opening file for writing: %v\n", err)
				return
			}
			defer fh.Close()

			_, err = fh.Write([]byte(lastGeneratedMessage))
			if err != nil {
				fmt.Fprintf(p.logForm, "error: writing message to file: %v\n", err)
				return
			}

			fmt.Fprintf(p.logForm, "info: succesfully wrote message to file: %v\n", file)

		})

	return p.flex
}

func (t *tui) console(app *tview.Application) tview.Primitive {

	// pageMessage is a struct for holding all the main forms and
	// views used in the message slide, so we can easily reference
	// them later in the code.
	type pageMessage struct {
		flex       *tview.Flex
		selectForm *tview.Form
		outputForm *tview.TextView
	}

	p := pageMessage{}

	p.selectForm = tview.NewForm()
	p.selectForm.SetBorder(true).SetTitle("select").SetTitleAlign(tview.AlignLeft)

	p.outputForm = tview.NewTextView()
	p.outputForm.SetBorder(true).SetTitle("output").SetTitleAlign(tview.AlignLeft)
	p.outputForm.SetChangedFunc(func() {
		// Will cause the log window to be redrawn as soon as
		// new output are detected.
		app.Draw()
	})

	// Create a flex layout.
	//
	// Create the outer flex layout.
	p.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		// Add a flex for the top windows with columns.
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(p.selectForm, 0, 3, false).

			// Add the message output form.
			AddItem(p.outputForm, 0, 10, false),

			0, 10, false)
		// Add a flex for the bottom log window.

	// Add items.

	// Create nodes dropdown field.
	nodesList, err := getNodeNames("nodeslist.cfg")
	if err != nil {
		fmt.Fprintf(p.outputForm, "error: failed to open nodeslist.cfg file\n")
	}

	nodesDropdown := tview.NewDropDown()
	nodesDropdown.SetLabelColor(tcell.ColorIndianRed)
	nodesDropdown.SetLabel("nodes").SetOptions(nodesList, nil)
	p.selectForm.AddFormItem(nodesDropdown)

	msgsValues := getMessageNames(p.outputForm)

	msgDropdown := tview.NewDropDown()
	msgDropdown.SetLabelColor(tcell.ColorIndianRed)
	msgDropdown.SetLabel("message").SetOptions(msgsValues, nil)
	p.selectForm.AddFormItem(msgDropdown)

	// Add button for manually updating dropdown menus.
	p.selectForm.AddButton("update dropdown menus", func() {
		nodesList, err := getNodeNames("nodeslist.cfg")
		if err != nil {
			fmt.Fprintf(p.outputForm, "error: failed to open nodeslist.cfg file\n")
		}
		nodesDropdown.SetLabel("nodes").SetOptions(nodesList, nil)

		msgsValues := getMessageNames(p.outputForm)
		msgDropdown.SetLabel("message").SetOptions(msgsValues, nil)
	})

	// Update the dropdown menus when the flex view gets focus.
	p.flex.SetFocusFunc(func() {
		nodesList, err := getNodeNames("nodeslist.cfg")
		if err != nil {
			fmt.Fprintf(p.outputForm, "error: failed to open nodeslist.cfg file\n")
		}
		nodesDropdown.SetLabel("nodes").SetOptions(nodesList, nil)

		msgsValues := getMessageNames(p.outputForm)
		msgDropdown.SetLabel("message").SetOptions(msgsValues, nil)
	})

	p.selectForm.AddButton("send message", func() {
		// here........

		nr, msgFileName := msgDropdown.GetCurrentOption()
		if nr < 1 {
			fmt.Fprintf(p.outputForm, "info: please select a message from the dropdown: %v\n", msgFileName)
			return
		}

		nr, toNode := nodesDropdown.GetCurrentOption()
		if nr < 1 {
			fmt.Fprintf(p.outputForm, "info: please select a message from the dropdown: %v\n", msgFileName)
			return
		}

		// fmt.Fprintf(p.outputForm, "info: nr=%v, text=%v\n", nr, text)

		filePath := filepath.Join("messages", msgFileName)
		fh, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(p.outputForm, "error: failed to open message file: %v\n", err)
			return
		}
		defer fh.Close()

		fileContent, err := io.ReadAll(fh)
		if err != nil {
			fmt.Fprintf(p.outputForm, "error: failed to read message file: %v\n", err)
			return
		}

		var msgs []Message
		err = json.Unmarshal(fileContent, &msgs)
		if err != nil {
			fmt.Fprintf(p.outputForm, "error: json unmarshal of file content failed: %v\n", err)
			return
		}

		msg := msgs[0]
		msg.FromNode = t.nodeName
		msg.ToNode = Node(toNode)

		// fmt.Fprintf(p.outputForm, "%#v\n", msg)

		sam, err := newSubjectAndMessage(msg)
		if err != nil {
			fmt.Fprintf(p.outputForm, "error: newSubjectAndMessage failed: %v\n", err)
			return
		}

		sams := []subjectAndMessage{sam}

		t.toRingbufferCh <- sams

	})

	go func() {
		for {
			select {
			case messageData := <-t.toConsoleCh:
				for _, v := range messageData {
					fmt.Fprintf(p.outputForm, "%v", v)
				}
			case <-t.ctx.Done():
				log.Printf("info: stopped tui toConsole worker\n")
				return
			}
		}
	}()

	return p.flex
}

// ---------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------

// getMessageNames will get the names of all the messages in
// the messages folder.
func getMessageNames(outputForm *tview.TextView) []string {
	// Create messages dropdown field.
	fInfo, err := ioutil.ReadDir("messages")
	if err != nil {
		fmt.Fprintf(outputForm, "error: failed to read files from messages dir\n")
	}

	msgsValues := []string{}
	msgsValues = append(msgsValues, "")

	for _, v := range fInfo {
		msgsValues = append(msgsValues, v.Name())
	}

	return msgsValues
}

// stringToSlice will Split the comma separated string
// into a and remove the start and end ampersand.
func stringToSlice(s string) (*[]string, error) {
	if s == "" {
		return nil, nil
	}

	var stringSlice []string

	sp := strings.Split(s, ",")

	for _, v := range sp {
		// Check if format is correct, return if not.
		pre := strings.HasPrefix(v, "\"")
		suf := strings.HasSuffix(v, "\"")
		if !pre || !suf {
			return nil, fmt.Errorf("stringToSlice: missing leading or ending ampersand")
		}
		// Remove leading and ending ampersand.
		v = v[1:]
		v = strings.TrimSuffix(v, "\"")

		stringSlice = append(stringSlice, v)
	}

	return &stringSlice, nil
}

// stringToNodes will Split the comma separated slice
// of nodes, and remove the start and end ampersand.
func stringToNode(s string) (*[]Node, error) {
	if s == "" {
		return nil, nil
	}

	var nodeSlice []Node

	sp := strings.Split(s, ",")

	for _, v := range sp {
		// Check if format is correct, return if not.
		pre := strings.HasPrefix(v, "\"")
		suf := strings.HasSuffix(v, "\"")
		if !pre || !suf {
			return nil, fmt.Errorf("stringToSlice: missing leading or ending ampersand")
		}
		// Remove leading and ending ampersand.
		v = v[1:]
		v = strings.TrimSuffix(v, "\"")

		nodeSlice = append(nodeSlice, Node(v))
	}

	return &nodeSlice, nil
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

	fh, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error: tui: you should create a file named nodeslist.cfg with all your nodes : %v", err)
	}
	defer fh.Close()

	nodes := []string{}
	// append a blank node at the beginning of the slice, so the dropdown
	// can be set to blank
	nodes = append(nodes, "")

	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		node := scanner.Text()
		nodes = append(nodes, node)
	}

	return nodes, nil
}

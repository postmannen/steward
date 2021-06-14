package steward

import (
	"bufio"
	"flag"
	"fmt"
	"os"

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
	err := console()
	if err != nil {
		return fmt.Errorf("error: console failed: %v", err)
	}

	return nil
}

// ---------------------------------------------------

func console() error {
	app := tview.NewApplication()

	nodeListForm := tview.NewList().ShowSecondaryText(false)
	nodeListForm.SetBorder(true).SetTitle("nodes").SetTitleAlign(tview.AlignLeft)
	inputCapture := func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Stop()
		}

		return event
	}
	nodeListForm.SetInputCapture(inputCapture)

	reqListForm := tview.NewList().ShowSecondaryText(false)
	reqListForm.SetBorder(true).SetTitle("methods").SetTitleAlign(tview.AlignLeft)
	reqFillForm := tview.NewForm()
	reqFillForm.SetBorder(true).SetTitle("Request values").SetTitleAlign(tview.AlignLeft)

	nodeListForm.SetSelectedFunc(func(i int, pri string, sec string, ru rune) {
		app.SetFocus(reqListForm)
	})

	// Create a flex layout.
	flexContainer := tview.NewFlex().
		AddItem(nodeListForm, 0, 1, true).
		AddItem(reqListForm, 0, 1, false).
		AddItem(reqFillForm, 0, 2, false)

	// Get nodes from file.
	nodes, err := getNodeNames("nodeslist.cfg")
	if err != nil {
		return err
	}

	// Selected func for node list.
	selectedFuncNodes := func() {
		var m Method
		ma := m.GetMethodsAvailable()

		// Select func to create the reqFillForm when req is selected in the reqListForm.
		selectedFuncReqList := func() {
			currentItem := reqListForm.GetCurrentItem()
			currentItemText, _ := reqListForm.GetItemText(currentItem)
			reqFillForm.AddButton(fmt.Sprintf("%v", currentItemText), nil)
			reqFillForm.AddButton("back", func() {
				reqFillForm.Clear(true)
				app.SetFocus(reqListForm)
			})

			inputCapture := func(event *tcell.EventKey) *tcell.EventKey {
				if event.Key() == tcell.KeyEscape {
					app.SetFocus(nodeListForm)
					reqFillForm.Clear(true)
					reqListForm.Clear()
				}

				return event
			}

			reqFillForm.SetInputCapture(inputCapture)
			app.SetFocus(reqFillForm)
		}

		// Add req items to the req list
		for k := range ma.methodhandlers {
			reqListForm.AddItem(string(k), "", rune(0), selectedFuncReqList)
		}
	}

	// Add nodes to the node list form.
	for _, v := range nodes {
		nodeListForm.AddItem(v, "", rune(0), selectedFuncNodes)
	}

	if err := app.SetRoot(flexContainer, true).Run(); err != nil {
		panic(err)
	}

	return nil
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

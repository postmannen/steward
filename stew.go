package steward

import (
	"flag"
	"fmt"
	"os"

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

func console() error {
	app := tview.NewApplication()

	nodes1 := tview.NewList().ShowSecondaryText(false)
	nodes1.SetBorder(true).SetTitle("nodes1")

	form1 := tview.NewForm()
	form1.SetBorder(true).SetTitle("form1").SetTitleAlign(tview.AlignLeft)

	nodes1.SetSelectedFunc(func(i int, pri string, sec string, ru rune) {
		app.SetFocus(form1)
	})

	// Create a flex to hold the nodes lists
	flex := tview.NewFlex().AddItem(nodes1, 0, 2, true).AddItem(form1, 0, 2, false)

	ships := []string{"ship1", "ship2", "ship3"}

	for _, v := range ships {
		nodes1.AddItem(v, "", rune(0), nodesSelected(nodes1, form1, app))
	}

	if err := app.SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}

	return nil
}

func nodesSelected(nodes1 *tview.List, form1 *tview.Form, app *tview.Application) func() {
	shipsdb := map[string]string{
		"ship1": "ship one",
		"ship2": "ship two",
		"ship3": "ship three",
	}

	f := func() {
		index := nodes1.GetCurrentItem()
		text, _ := nodes1.GetItemText(index)
		form1.Clear(true)
		// selected set to nil
		form1.AddButton(fmt.Sprintf("%v", shipsdb[text]), nil)
		form1.AddButton("back", func() {
			form1.Clear(true)
			app.SetFocus(nodes1)
		})
	}

	return f
}

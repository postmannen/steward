package stew

import (
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

type slide struct {
	name      string
	key       tcell.Key
	primitive tview.Primitive
}

func (s *Stew) Start() error {
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
	// elements. If adding a new slide, make sure that slides are ordererin
	// chronological order, so we can auto generate the info menu with it's
	// corresponding F key based on the slice index+1.
	slides := []slide{
		{name: "info", key: tcell.KeyF1, primitive: infoSlide(app)},
		{name: "message", key: tcell.KeyF2, primitive: messageSlide(app)},
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

	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	return nil
}

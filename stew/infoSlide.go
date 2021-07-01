package stew

import (
	"fmt"

	"github.com/rivo/tview"
)

func infoSlide(app *tview.Application) tview.Primitive {
	flex := tview.NewFlex()
	flex.SetTitle("info")
	flex.SetBorder(true)

	textView := tview.NewTextView()
	flex.AddItem(textView, 0, 1, false)

	fmt.Fprintf(textView, "Information page for Stew.\n")

	return flex
}

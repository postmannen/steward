package stew

import "github.com/rivo/tview"

func infoSlide(app *tview.Application) tview.Primitive {
	flex := tview.NewFlex()
	flex.SetTitle("info")
	flex.SetBorder(true)

	return flex
}

package main

import (
	"log"

	"github.com/egoist/mygo"
)

func main() {
	mygo.Bind(&Rokit{})

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:           "Rokit Audio Editor",
			Width:           1100,
			Height:          720,
			MinWidth:        900,
			MinHeight:       600,
			TitleBarStyle:   mygo.TitleBarHidden, // the page draws its own title area, like the canvas design
			TitleBarHeight:  40,
			BackgroundColor: "#F5F5F7", // --bg in src/style.css
			StateKey:        "main",
			URL:             "/",
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

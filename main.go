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
			Width:           720,
			Height:          880,
			MinWidth:        480,
			MinHeight:       560,
			BackgroundColor: "#F4F6F8", // --bg in src/style.css
			StateKey:        "main",
			URL:             "/",
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"fmt"
	"log"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy"
)

func main() {
	a := app.NewWithID("com.github.hippodribble.seismic.segytest")
	fmt.Println(a.Metadata())
	uri := storage.NewFileURI("/Users/glenn/Documents/Data/Seismic Data/Mobil AVO Viking Graben/seismic.segy")
	fmt.Println(uri.Path())
	f, err := segy.Open(uri)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("sample format code: %d", f.BinaryHeader.DataSampleFormatCode)
	log.Printf("samples/trace (binary default): %d", f.BinaryHeader.SamplesPerTrace)

	catalog, err := f.BuildTraceCatalog()
	if err != nil {
		log.Fatal(err)
	}

	// Number of indexed traces
	count := len(catalog.Entries)
	fmt.Println(count, "traces found in catalog")
	summary := catalog.HeaderRangeSummary()
	fmt.Println(summary)
}

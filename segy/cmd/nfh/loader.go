package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy"
)

func loadTestFile() {

	uri := storage.NewFileURI("/Users/glenn/Documents/Data/JOGMEC Tosa/NFH Data/a.segy")
	f, err := segy.OpenSEGYWithByteOrderAndFormat(uri, binary.BigEndian, 1)
	if err != nil {
		fmt.Println(err)
		return
	}

	log.Printf("sample format code: %d", f.BinaryHeader.DataSampleFormatCode)
	log.Printf("samples/trace (binary default): %d", f.BinaryHeader.SamplesPerTrace)

	catalog, err := f.BuildTraceCatalog()
	if err != nil {
		fmt.Println("error building trace catalog: " + err.Error())
		return
	}

	fields := []segy.CustomTraceField{
		{Name: "Volume", Offset: 203, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "Pressure", Offset: 193, Type: segy.TraceHeaderValueTypeUint32},
		{Name: "Gun", Offset: 197, Type: segy.TraceHeaderValueTypeInt32},
		{Name: "Subarray", Offset: 201, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "Gunmask", Offset: 173, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "Sourcefiring", Offset: 171, Type: segy.TraceHeaderValueTypeInt16},
	}

	traceHeader := catalog.Entries[0].Header

	headerValues, err := segy.ReadCustomTraceValuesFromBytes(
		traceHeader.Raw[:],
		fields,
		binary.BigEndian,
	)
	if err != nil {
		statusChannel <- "error reading custom trace values: " + err.Error()
		return
	}

	for k, v := range headerValues {
		fmt.Println(k, v)
	}
}

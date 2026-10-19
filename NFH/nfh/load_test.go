package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"testing"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy"
)

func TestXxx(t *testing.T) {
	ap := app.New()
	fmt.Println(ap.Settings())
	uri := storage.NewFileURI("/Users/glenn/Documents/Data/JOGMEC Tosa/NFH Data/a.segy")
	f, err := segy.OpenSEGYWithByteOrderAndFormat(uri, binary.BigEndian, 1)

	if err != nil {
		t.Fatalf("error opening SEGY file: %s", err)
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
		{Name: "ep17", Offset: 17, Type: segy.TraceHeaderValueTypeInt32},
		{Name: "Sequence", Offset: 169, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "SourceActive", Offset: 171, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "GunMask", Offset: 173, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "ArrayVolume", Offset: 175, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "Pressure", Offset: 193, Type: segy.TraceHeaderValueTypeInt32},
		{Name: "Gun", Offset: 197, Type: segy.TraceHeaderValueTypeInt32},
		{Name: "SubArray", Offset: 201, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "GunVolume", Offset: 203, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "VWater", Offset: 211, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "CMG", Offset: 213, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "ScaleElevation", Offset: 69, Type: segy.TraceHeaderValueTypeInt16},
		{Name: "ScaleCoordinates", Offset: 71, Type: segy.TraceHeaderValueTypeInt16},
	}

	for i := range catalog.Entries {

		traceHeader := catalog.Entries[i].Header
		headerValues, err := segy.ReadCustomTraceValuesFromBytes(
			traceHeader.Raw[:],
			fields,
			binary.BigEndian,
		)
		if err != nil {
			t.Fatalf("error reading custom trace values: %s", err)
			return
		}
		fmt.Println("---")
		for k, v := range headerValues {
			fmt.Println(k, v)
		}
	}
	e:=catalog.Entries[0]
	th, samples, err := f.ReadDecodedTraceFromIndex(e)
	if err != nil {
		t.Fatalf("error reading decoded trace: %s", err)
		return
	}

	shifted, err := segy.ShiftTraceBySamplesFrequencyDomain(samples, -60)
	if err != nil {
		t.Fatalf("error shifting trace: %s", err)
		return
	}
	for i:=range 100{
		fmt.Println(float64(th.SampleIntervalMicrosec)/1000.0*float64(i), samples[i],shifted[i])
	}
}

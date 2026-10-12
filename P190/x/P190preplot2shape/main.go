package main

import (
	"flag"
	"log"
	"os"

	"github.com/hippodribble/seismic/P190"
)

var rectype string
var types = []string{"S", "G", "Q", "A", "T", "V", "E", "Z", "C", "auto"}

func main() {
	flag.StringVar(&rectype, "r", "auto", "Record Type - usually one of S, G, Q, A, T, V, E, Z, C - use 'auto' to determine from file")

	flag.Parse()

	typefound := false
	for _, thistype := range types {
		if thistype == rectype {
			typefound = true
		}
	}
	if !typefound {
		log.Fatalln("Use one of S, G, Q, A, T, V, E, Z, C, auto as record type.", rectype, "is not known.")
	}

	filename := os.Args[len(os.Args)-1]
	p1 := P190.NewP190File(filename, rectype)
	err := p1.Load()
	if err != nil {
		log.Fatalln("Error loading P190 file:", err)
	}
	p1.ToShapeFile(filename)
}

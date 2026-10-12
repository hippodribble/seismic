package P190

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jonas-p/go-shp"
)

// A P190 NodeRecord represents a single node (non-receiver) record in a P190 file, containing information about a point in a preplot line.
// Preplots are generally defined using a node record of some kind (S,V,Z etc).
type NodeRecord struct {
	Type                          string
	LineName                      string
	Spare                         string
	VesselID, SourceID, TBotherID string
	Point                         int
	Easting, Northing             float64
}

func (n *NodeRecord) FromP190Text(s string) error {
	n.Type = string(s[0])
	n.LineName = strings.TrimSpace(s[1:13])
	p, err := strconv.Atoi(strings.TrimSpace(s[19:25]))
	if err != nil {
		return fmt.Errorf("bad record: %s", s)
	}
	n.Point = p
	x, err := strconv.ParseFloat(strings.TrimSpace(s[46:55]), 64)
	if err != nil {
		return fmt.Errorf("bad record: %s", s)
	}
	n.Easting = x
	y, err := strconv.ParseFloat(strings.TrimSpace(s[55:64]), 64)
	if err != nil {
		return fmt.Errorf("bad record: %s", s)
	}
	n.Northing = y
	return nil
}

// Node represents a point in a preplot line with its coordinates and associated line name.
type PreplotLine struct {
	Name    string
	Records *[]NodeRecord
}

// NewPreplotLine creates a new PreplotLine instance with the given name.
func NewPreplotLine(name string) PreplotLine {
	l := PreplotLine{Name: name}
	return l
}

// P190File represents a P190 file with its name, type, and lines of preplot data.
type P190File struct {
	Name           string
	preplotrectype string
	Lines          []PreplotLine
}

// NewP190File creates a new P190File instance with the given name and preplot record type.
func NewP190File(name, preplotrectype string) P190File {
	p1 := P190File{
		Name:           name,
		preplotrectype: preplotrectype,
		Lines:          []PreplotLine{},
	}
	return p1
}

// Load reads the P190 file and populates the lines of preplot data.
func (p *P190File) Load() error {
	f, err := os.Open(p.Name)
	if err != nil {
		return fmt.Errorf("no such file: %s", p.Name)
	}
	if p.preplotrectype == "auto" {
		b,err:=io.ReadAll(f)
		if err!=nil{
			return fmt.Errorf("error reading file: %s", p.Name)
		}
		p.preplotrectype = MostCommonPrefix(string(b))
		f.Seek(0,0)
	}
	sc := bufio.NewScanner(f)

	lines := []PreplotLine{}
	lastline := "ZOIDMONSTER"

	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), p.preplotrectype) {
			continue
		}
		r := NodeRecord{}
		err := r.FromP190Text(sc.Text())
		if err != nil {
			return err
		}
		if r.LineName != lastline {
			l := NewPreplotLine(r.LineName)
			l.Records = &[]NodeRecord{}
			*l.Records = append((*l.Records), r)
			lines = append(lines, l)
			lastline = r.LineName
		} else {
			l := lines[len(lines)-1]
			*l.Records = append((*l.Records), r)
		}
	}
	p.Lines = lines
	if sc.Err() != nil {
		return fmt.Errorf("error reading file: %s", p.Name)
	}
	return nil
}

// ToShapeFile converts the P190 file data into an ESRI shapefile with the specified filename.
//
// The writer is open source, viz github.com/jonas-p/go-shp
func (p P190File) ToShapeFile(filename string) error {
	if len(p.Lines) == 0 {
		return fmt.Errorf("no lines to convert to shapefile")
	}

	shape, err := shp.Create(filename+".shp", shp.POLYLINE)
	if err != nil {
		return fmt.Errorf("cannot create shapefile %s", filename)
	}
	defer shape.Close()
	log.Println("Created shape file", filename+".shp")
	counter := 0

	for _, l := range p.Lines {
		fields := []shp.Field{
			shp.StringField("LINE", 32),
		}
		shape.SetFields(fields)

		points := []shp.Point{}
		for _, point := range *l.Records {

			points = append(points, shp.Point{X: point.Easting, Y: point.Northing})
		}
		partial := [][]shp.Point{points}
		polyline := shp.NewPolyLine(partial)
		shape.Write(polyline)
		shape.WriteAttribute(counter, 0, l.Name)
		counter++
		log.Println(counter, "tracks created")

	}
	shape.Close()
	err = os.Rename(filename+"dbf", filename+".dbf")
	if err != nil {
		return fmt.Errorf("cannot rename dbf file: %s", err)
	}

	return nil
}

// MostCommonPrefix returns the most common prefix character in the given text, ignoring lines that start with a space or 'H'.
//
// This is useful because P190 preplot files are identified by a specific record type, but not always the same one.
func MostCommonPrefix(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}

	prefixmap := make(map[string]int)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if string(line[0]) == " " {
			continue
		}
		if string(line[0]) == "H" {
			continue
		}
		prefixmap[string(line[0])]++
	}

	maxCount := 0
	var mostCommon string
	for k, v := range prefixmap {
		if v > maxCount {
			maxCount = v
			mostCommon = k
		}
	}
	return mostCommon

}

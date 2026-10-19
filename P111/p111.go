package p111

import (
	"bufio"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/hippodribble/drawing/layermanager"
)

type SP struct {
	Point    int
	Location layermanager.Point
}

type P111File struct {
	LineName string
	SPs      []SP
}

func LoadSPsFromP111File(filepath string,ID string) (string, []SP, error) {
	r, err := os.Open(filepath)
	if err != nil {
		return "", nil, err
	}
	defer r.Close()

	var linename string

	sc := bufio.NewScanner(r)
	var sps []SP
	for sc.Scan() {
		t := sc.Text()
		if len(t)==0{continue}
		if t[0] == 'P' {
			cols := strings.Split(t, ",")
			if cols[9]!=ID{continue}
			if len(sps)==0{linename = cols[2] }
			sx, err := strconv.ParseFloat(cols[12], 64)
			if err != nil {
				return "", nil, errors.New("bad SX value in " + t)
			}
			sy, err := strconv.ParseFloat(cols[13], 64)
			if err != nil {
				return "", nil, errors.New("bad SY value in " + t)
			}
			spnum, err := strconv.Atoi(cols[4])
			if err != nil {
				log.Fatalln("Possible error in P1 file: SP number is not an integer", t)
			}
			sps = append(sps, SP{
				Point:    spnum,
				Location: layermanager.Point{X: sx, Y: sy},
			})
		}
	}
	if err := sc.Err(); err != nil {
		return "", nil, err
	}
	return linename, sps, nil
}

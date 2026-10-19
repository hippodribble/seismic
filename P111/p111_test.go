package p111

import "testing"

func TestP111Reader(t *testing.T) {
	filename := "/Users/glenn/Documents/Data/Hidaka/Navigation/P111/2026012-001.p111"
	linename, sps, err := LoadSPsFromP111File(filename,"S02")
	if err != nil {
		t.Fatalf("Failed to load SPs from P111 file: %v", err)
	}
	t.Logf("Line name: %s", linename)
	t.Logf("Number of SPs: %d", len(sps))
}

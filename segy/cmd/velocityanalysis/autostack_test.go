package main

import "testing"

func TestShouldAutoStackAfterPickEdit(t *testing.T) {
	cases := []struct {
		name          string
		previousCount int
		newCount      int
		dragCompleted bool
		want          bool
	}{
		{name: "new pick without drag does not trigger", previousCount: 3, newCount: 4, dragCompleted: false, want: false},
		{name: "drag release triggers", previousCount: 3, newCount: 4, dragCompleted: true, want: true},
		{name: "delete triggers", previousCount: 4, newCount: 3, dragCompleted: false, want: true},
		{name: "drag release with no count change triggers", previousCount: 4, newCount: 4, dragCompleted: true, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAutoStackAfterPickEdit(tc.previousCount, tc.newCount, tc.dragCompleted); got != tc.want {
				t.Fatalf("shouldAutoStackAfterPickEdit(%d, %d, %v) = %v, want %v", tc.previousCount, tc.newCount, tc.dragCompleted, got, tc.want)
			}
		})
	}
}

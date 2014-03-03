package cobe

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestShortLearn(t *testing.T) {
	filename, err := tmpCopy("data/pg11.brain")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(filename)

	b, err := OpenBrain(filename)
	if err != nil {
		t.Fatal(err)
	}

	b.Learn("cobe cobe cobe")

	r := b.Reply("cobe")
	if strings.Index(r, "cobe") != -1 {
		t.Fatalf("incorrectly learned cobe: %s", r)
	}
}

func TestReply(t *testing.T) {
	filename, err := tmpCopy("data/pg11.brain")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(filename)

	b, err := OpenBrain(filename)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("End reply: %s\n", b.Reply("this this this is a test test test"))
}

func TestToEdges(t *testing.T) {
	tests := []struct {
		order    int
		tokenIds []tokenID
		expected []edge
	}{
		{3,
			[]tokenID{1, 1, 1, 2, -1, 3, -1, 4, -1, 5, 1, 1, 1},
			[]edge{
				{[]tokenID{1, 1, 1}, []tokenID{1, 1, 2}, false},
				{[]tokenID{1, 1, 2}, []tokenID{1, 2, 3}, true},
				{[]tokenID{1, 2, 3}, []tokenID{2, 3, 4}, true},
				{[]tokenID{2, 3, 4}, []tokenID{3, 4, 5}, true},
				{[]tokenID{3, 4, 5}, []tokenID{4, 5, 1}, false},
				{[]tokenID{4, 5, 1}, []tokenID{5, 1, 1}, false},
				{[]tokenID{5, 1, 1}, []tokenID{1, 1, 1}, false},
			},
		},
	}

	for tn, tt := range tests {
		edges := toEdges(tt.order, tt.tokenIds)

		for i := 0; i < len(tt.expected); i++ {
			if len(edges) != len(tt.expected) {
				t.Errorf("[%d] bad edge count: %d != %d",
					len(edges), len(tt.expected))
			}

			if !edgeEqual(edges[i], tt.expected[i]) {
				t.Errorf("[%d] bad edge: %v != %v", tn,
					edges[i], tt.expected[i])
			}
		}
	}
}

func nodeEqual(a []tokenID, b []tokenID) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func edgeEqual(a edge, b edge) bool {
	return nodeEqual(a.prev, b.prev) && nodeEqual(a.next, b.next) &&
		a.hasSpace == b.hasSpace
}
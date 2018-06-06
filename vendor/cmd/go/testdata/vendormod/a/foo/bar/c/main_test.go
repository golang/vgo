package c

import (
	"os"
	"testing"
)

func TestDir(t *testing.T) {
	if _, err := os.Stat("../../../testdata/1"); err != nil {
		t.Fatalf("testdata: %v", err)
	}
	if _, err := os.Stat("./testdata/1"); err != nil {
		t.Fatalf("testdata: %v", err)
	}
}

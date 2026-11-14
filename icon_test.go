package main

import (
	"bytes"
	"testing"
	"unsafe"
)

func TestEmbeddedIcon(t *testing.T) {
	loadIcon()
	if iconW != 2048 || iconH != 2048 || len(iconPix) != 2048*2048*4 {
		t.Fatalf("icon %dx%d pix %d", iconW, iconH, len(iconPix))
	}
	var ink bool
	for i := 0; i < len(iconPix); i += 4 {
		if iconPix[i+3] != 0 {
			ink = true
			break
		}
	}
	if !ink {
		t.Fatal("icon decoded empty")
	}
	p, w, h := iconABI()
	if p == nil || w != iconW || h != iconH {
		t.Fatalf("icon ABI %v %dx%d", p, w, h)
	}
	if uintptr(p) == uintptr(unsafe.Pointer(&iconPix[0])) {
		t.Fatal("icon must be C.malloc, not Go heap")
	}
	if !bytes.Equal(unsafe.Slice((*byte)(p), w*h*4), iconPix) {
		t.Fatal("icon pixels mismatch")
	}
}

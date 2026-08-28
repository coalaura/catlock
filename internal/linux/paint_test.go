//go:build linux

package linux

import "testing"

func TestMaskedComponentScalesIntoMask(t *testing.T) {
	actual := maskedComponent(0xff, 0x00ff0000)
	if actual != 0x00ff0000 {
		t.Fatalf("full red component = %#08x", actual)
	}

	actual = maskedComponent(0, 0x0000ff00)
	if actual != 0 {
		t.Fatalf("zero green component = %#08x", actual)
	}

	actual = maskedComponent(0xff, 0x0000f800)
	if actual != 0x0000f800 {
		t.Fatalf("full 5-bit component = %#08x", actual)
	}
}

// iconextract/decode_test.go
package iconextract

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildFixture constructs a synthetic classic-.uexp-shaped byte buffer:
// [junk][int32 len-prefix]["PF_B8G8R8A8"][null][4 reserved bytes]
// [int32 mip count][pad][mip0 pixel bytes][filler for smaller mips].
// pad's *length* (not content) and filler's *length* both matter -- they
// stand in for the real per-mip header bytes and the smaller mips' pixel
// bytes respectively, which Decode's byte-accounting must skip/ignore
// without needing to know their real internal structure.
func buildFixture(dim int, pad, mip0 []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("junkjunk")

	marker := "PF_B8G8R8A8"
	lenPrefix := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenPrefix, uint32(len(marker)+1))
	buf.Write(lenPrefix)
	buf.WriteString(marker)
	buf.WriteByte(0)

	buf.Write(make([]byte, 4)) // reserved

	mipCount := 0
	for d := dim; d >= 1; d /= 2 {
		mipCount++
	}
	mipCountBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(mipCountBytes, uint32(mipCount))
	buf.Write(mipCountBytes)

	buf.Write(pad)
	buf.Write(mip0)

	// filler bytes standing in for every smaller mip's pixel data, so
	// Decode's total-remaining-minus-total-payload arithmetic works out
	// to exactly len(pad).
	d := dim
	for m := 1; m < mipCount; m++ {
		d /= 2
		buf.Write(make([]byte, d*d*4))
	}
	return buf.Bytes()
}

func TestDecodeExtracts2x2Icon(t *testing.T) {
	pad := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11} // 7 arbitrary header padding bytes
	mip0 := []byte{
		10, 20, 30, 255, // (0,0): B,G,R,A
		40, 50, 60, 255, // (1,0)
		70, 80, 90, 128, // (0,1)
		100, 110, 120, 0, // (1,1)
	}
	fixture := buildFixture(2, pad, mip0)

	img, err := Decode(fixture)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatalf("image size = %dx%d, want 2x2", img.Bounds().Dx(), img.Bounds().Dy())
	}
	c := img.NRGBAAt(0, 0)
	if c.R != 30 || c.G != 20 || c.B != 10 || c.A != 255 {
		t.Errorf("pixel (0,0) = %+v, want R=30 G=20 B=10 A=255", c)
	}
	c = img.NRGBAAt(1, 1)
	if c.R != 120 || c.G != 110 || c.B != 100 || c.A != 0 {
		t.Errorf("pixel (1,1) = %+v, want R=120 G=110 B=100 A=0", c)
	}
}

func TestDecodeRejectsMissingPixelFormatMarker(t *testing.T) {
	fixture := []byte("some random bytes with no recognized pixel format marker anywhere in them at all")
	if _, err := Decode(fixture); err == nil {
		t.Fatal("expected an error when no PF_B8G8R8A8 marker is present")
	}
}

func TestDecodeRejectsTruncatedFile(t *testing.T) {
	pad := []byte{}
	mip0 := []byte{1, 2, 3, 4} // only 1 pixel's worth, but dim=2 needs 4
	fixture := buildFixture(2, pad, mip0)
	fixture = fixture[:len(fixture)-8] // truncate away the last mip's filler AND part of mip0
	if _, err := Decode(fixture); err == nil {
		t.Fatal("expected an error for a truncated file")
	}
}

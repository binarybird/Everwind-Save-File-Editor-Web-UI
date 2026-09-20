// iconextract/decode_test.go
package iconextract

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildFixture constructs a synthetic classic-.uexp-shaped byte buffer:
// [junk][int32 len-prefix]["PF_B8G8R8A8"][null][4 reserved bytes]
// [int32 mip count][mip0 pixel bytes][filler for smaller mips]. Mip 0's
// pixel bytes start immediately after the mip count field -- filler's
// *length* (not content) stands in for the smaller mips' pixel data,
// which Decode never reads but which affects nothing here since Decode
// only reads dim*dim*4 bytes starting right after the mip count field.
func buildFixture(dim int, mip0 []byte) []byte {
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

	buf.Write(mip0)

	// filler bytes standing in for every smaller mip's pixel data -- not
	// read by Decode, just makes the fixture's overall shape realistic.
	d := dim
	for m := 1; m < mipCount; m++ {
		d /= 2
		buf.Write(make([]byte, d*d*4))
	}
	return buf.Bytes()
}

func TestDecodeExtracts2x2Icon(t *testing.T) {
	mip0 := []byte{
		10, 20, 30, 255, // (0,0): B,G,R,A
		40, 50, 60, 255, // (1,0)
		70, 80, 90, 128, // (0,1)
		100, 110, 120, 0, // (1,1)
	}
	fixture := buildFixture(2, mip0)

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

// TestDecodeDoesNotSkipMip0Bytes is a regression test for a real bug: an
// earlier version of Decode computed pixelStart by subtracting the full
// mip chain's total byte count from the bytes remaining after the pixel
// format string, on the theory that some header block preceded all the
// pixel data. That header block doesn't exist -- mip 0's pixel bytes
// start immediately after the mip-count field -- so the old formula
// skipped real leading pixel bytes (which happened to be zero/
// transparent for the two icons sampled during this feature's design
// spike, masking the bug) and, reading a same-sized window starting
// late, wrapped into the next mip's data at the tail. This test uses a
// mip0 with distinctive, non-zero leading bytes specifically so a
// too-late pixelStart would be caught immediately.
func TestDecodeDoesNotSkipMip0Bytes(t *testing.T) {
	dim := 4
	mip0 := make([]byte, dim*dim*4)
	for i := range mip0 {
		mip0[i] = byte(i + 1) // 1, 2, 3, ... -- no byte is 0, so any
		// off-by-N-bytes read shows up as a value mismatch immediately,
		// including at pixel (0,0), which a header-skipping bug would
		// have zeroed out or shifted.
	}
	fixture := buildFixture(dim, mip0)

	img, err := Decode(fixture)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	c := img.NRGBAAt(0, 0)
	// mip0[0:4] = B=1,G=2,R=3,A=4 -> NRGBA R=3,G=2,B=1,A=4
	if c.R != 3 || c.G != 2 || c.B != 1 || c.A != 4 {
		t.Errorf("pixel (0,0) = %+v, want R=3 G=2 B=1 A=4 (decoded straight from mip0's first 4 bytes, no header skipped)", c)
	}
	last := img.NRGBAAt(dim-1, dim-1)
	lastIdx := (dim*dim - 1) * 4
	wantB, wantG, wantR, wantA := mip0[lastIdx], mip0[lastIdx+1], mip0[lastIdx+2], mip0[lastIdx+3]
	if last.R != wantR || last.G != wantG || last.B != wantB || last.A != wantA {
		t.Errorf("last pixel = %+v, want R=%d G=%d B=%d A=%d (mip0's true last 4 bytes, not wrapped into the next mip's filler)", last, wantR, wantG, wantB, wantA)
	}
}

func TestDecodeRejectsMissingPixelFormatMarker(t *testing.T) {
	fixture := []byte("some random bytes with no recognized pixel format marker anywhere in them at all")
	if _, err := Decode(fixture); err == nil {
		t.Fatal("expected an error when no PF_B8G8R8A8 marker is present")
	}
}

func TestDecodeRejectsTruncatedFile(t *testing.T) {
	mip0 := []byte{1, 2, 3, 4} // only 1 pixel's worth, but dim=2 needs 4
	fixture := buildFixture(2, mip0)
	fixture = fixture[:len(fixture)-8] // truncate away the last mip's filler AND part of mip0
	if _, err := Decode(fixture); err == nil {
		t.Fatal("expected an error for a truncated file")
	}
}

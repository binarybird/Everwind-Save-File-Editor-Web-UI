// iconextract/decode.go
package iconextract

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
)

// pixelFormatMarker is the only pixel format this decoder understands --
// UTexture2D's uncompressed 32-bit BGRA layout. A compressed format
// (DXT/BC-family) is reported as an error so callers can skip that
// asset. See docs/superpowers/specs/2026-09-19-inventory-tab-design.md,
// "Icon extraction pipeline" for the derivation of this algorithm.
const pixelFormatMarker = "PF_B8G8R8A8"

// Decode extracts the largest (mip 0) image from a classic-format
// UTexture2D .uexp file's raw bytes.
//
// The pixel format string is followed by a reserved int32 and a mip
// count (int32); a full square power-of-two mip chain down to 1x1 is
// assumed, so dim = 2^(mipCount-1). Mip 0's raw pixel bytes begin
// immediately after those two int32 fields -- verified directly against
// three real icons (different categories, all 32x32) by dumping the
// full remaining byte range as a continuous image strip and visually
// confirming the mip chain (32x32 followed by progressively smaller
// copies of the same art) starts exactly there.
//
// An earlier version of this function computed a "header size" via
// (bytes remaining) minus (the full mip chain's total pixel byte
// count), on the theory that some other per-mip header data preceded
// all the pixel bytes as one block. That arithmetic happened to
// coincidentally match two sample icons during this feature's design
// spike, but was wrong in general: the bytes it skipped past were
// themselves the image's own top rows (typically transparent, so the
// error wasn't obvious), and reading a same-sized window starting late
// meant the window's tail wrapped into the next mip's data -- a visible
// sliver of "foreign" content bleeding in at one edge. See
// docs/superpowers/sdd/2026-09-19-inventory-tab progress ledger for the
// live bug report and diagnosis that found this.
func Decode(uexp []byte) (*image.NRGBA, error) {
	idx := bytes.Index(uexp, []byte(pixelFormatMarker))
	if idx < 0 {
		return nil, fmt.Errorf("iconextract: pixel format %q not found (likely a compressed format, unsupported)", pixelFormatMarker)
	}
	if idx < 4 {
		return nil, fmt.Errorf("iconextract: pixel format string at offset %d has no room for its length prefix", idx)
	}
	lengthPrefix := int32(binary.LittleEndian.Uint32(uexp[idx-4 : idx]))
	if int(lengthPrefix) != len(pixelFormatMarker)+1 {
		return nil, fmt.Errorf("iconextract: unexpected pixel format string length prefix %d, want %d", lengthPrefix, len(pixelFormatMarker)+1)
	}
	after := idx + int(lengthPrefix)
	if after+8 > len(uexp) {
		return nil, fmt.Errorf("iconextract: file truncated after pixel format string")
	}
	mipCount := int32(binary.LittleEndian.Uint32(uexp[after+4 : after+8]))
	if mipCount < 1 || mipCount > 16 {
		return nil, fmt.Errorf("iconextract: implausible mip count %d", mipCount)
	}
	dim := 1 << uint(mipCount-1)

	pixelStart := after + 8
	pixelEnd := pixelStart + dim*dim*4
	if pixelEnd > len(uexp) {
		return nil, fmt.Errorf("iconextract: computed pixel range [%d:%d) exceeds file length %d", pixelStart, pixelEnd, len(uexp))
	}
	bgra := uexp[pixelStart:pixelEnd]

	img := image.NewNRGBA(image.Rect(0, 0, dim, dim))
	for i := 0; i < dim*dim; i++ {
		b, g, r, a := bgra[i*4], bgra[i*4+1], bgra[i*4+2], bgra[i*4+3]
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, a
	}
	return img, nil
}

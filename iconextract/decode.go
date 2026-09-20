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
// assumed, so dim = 2^(mipCount-1). The exact per-mip header layout
// isn't parsed field-by-field -- instead, the total header-block size is
// derived algebraically: (bytes remaining after the pixel format
// string) minus (the full mip chain's total pixel byte count) equals
// the header block, which precedes all pixel data. This was verified
// against two real icons (different categories, both 32x32) during this
// feature's design spike.
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

	totalPixelPayload := 0
	for m := int32(0); m < mipCount; m++ {
		mipDim := dim >> uint(m)
		totalPixelPayload += mipDim * mipDim * 4
	}
	totalRemaining := len(uexp) - after
	headerBytes := totalRemaining - totalPixelPayload
	if headerBytes < 0 {
		return nil, fmt.Errorf("iconextract: computed negative header size (%d); mip count %d / dim %d likely wrong for this file", headerBytes, mipCount, dim)
	}
	pixelStart := after + headerBytes
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

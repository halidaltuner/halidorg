package photo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// EXIF is what a photo form can take from a JPEG: camera, lens, exposure
// ("1/250 · f/8 · ISO 160") and when it was taken. Empty fields were not in
// the file.
type EXIF struct {
	Camera, Lens, Exposure string
	TakenAt                *time.Time
}

// EXIF tags read (TIFF 6.0 and EXIF 2.3).
const (
	tagMake         = 0x010F
	tagModel        = 0x0110
	tagExifIFD      = 0x8769
	tagExposureTime = 0x829A
	tagFNumber      = 0x829D
	tagISO          = 0x8827
	tagDateTimeOrig = 0x9003
	tagLensModel    = 0xA434
)

// ReadEXIF reads the EXIF block of a JPEG. It never fails on odd input: a
// file without EXIF, or with a broken one, gives what could be read.
func ReadEXIF(jpeg []byte) EXIF {
	var out EXIF
	app1 := findAPP1(jpeg)
	if app1 == nil {
		return out
	}
	t := tiff(app1)
	ifd0, ok := t.ifd(t.u32(4))
	if !ok {
		return out
	}
	// The model alone ("X-T5"), as the prototype; the make when there is
	// no model.
	out.Camera = t.str(ifd0[tagModel])
	if out.Camera == "" {
		out.Camera = t.str(ifd0[tagMake])
	}
	e, ok := ifd0[tagExifIFD]
	if !ok {
		return out
	}
	exif, ok := t.ifd(t.entryU32(e))
	if !ok {
		return out
	}
	out.Lens = t.str(exif[tagLensModel])
	var parts []string
	if n, d, ok := t.rational(exif[tagExposureTime]); ok && n > 0 && d > 0 {
		if n >= d {
			parts = append(parts, fmt.Sprintf("%gs", float64(n)/float64(d)))
		} else {
			parts = append(parts, fmt.Sprintf("1/%d", (d+n/2)/n))
		}
	}
	if n, d, ok := t.rational(exif[tagFNumber]); ok && d > 0 {
		parts = append(parts, fmt.Sprintf("f/%g", float64(int(float64(n)/float64(d)*10+0.5))/10))
	}
	if iso, ok := exif[tagISO]; ok {
		if v := t.entryShort(iso); v > 0 {
			parts = append(parts, fmt.Sprintf("ISO %d", v))
		}
	}
	out.Exposure = strings.Join(parts, " · ")
	if s := t.str(exif[tagDateTimeOrig]); s != "" {
		if when, err := time.Parse("2006:01:02 15:04:05", s); err == nil {
			out.TakenAt = &when
		}
	}
	return out
}

// findAPP1 returns the TIFF data of the JPEG's Exif APP1 segment.
func findAPP1(b []byte) []byte {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return nil
		}
		marker := b[i+1]
		if marker == 0xDA || marker == 0xD9 { // image data: no EXIF before it
			return nil
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return nil
		}
		seg := b[i+4 : i+2+n]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return seg[6:]
		}
		i += 2 + n
	}
	return nil
}

type tiffData struct {
	b  []byte
	bo binary.ByteOrder
}

type entry struct {
	typ, count uint32
	value      []byte // the 4 value bytes of the entry
}

func tiff(b []byte) tiffData {
	t := tiffData{b: b, bo: binary.BigEndian}
	if len(b) >= 2 && b[0] == 'I' && b[1] == 'I' {
		t.bo = binary.LittleEndian
	}
	return t
}

func (t tiffData) u32(off int) int {
	if off < 0 || off+4 > len(t.b) {
		return -1
	}
	return int(t.bo.Uint32(t.b[off:]))
}

// ifd reads the entries of the IFD at off, by tag.
func (t tiffData) ifd(off int) (map[uint16]entry, bool) {
	if off < 8 || off+2 > len(t.b) {
		return nil, false
	}
	n := int(t.bo.Uint16(t.b[off:]))
	out := map[uint16]entry{}
	for i := 0; i < n && i < 512; i++ {
		p := off + 2 + 12*i
		if p+12 > len(t.b) {
			break
		}
		out[t.bo.Uint16(t.b[p:])] = entry{typ: uint32(t.bo.Uint16(t.b[p+2:])), count: t.bo.Uint32(t.b[p+4:]), value: t.b[p+8 : p+12]}
	}
	return out, true
}

func (t tiffData) entryU32(e entry) int { return int(t.bo.Uint32(e.value)) }

func (t tiffData) entryShort(e entry) int {
	switch e.typ {
	case 3: // SHORT
		return int(t.bo.Uint16(e.value))
	case 4: // LONG
		return int(t.bo.Uint32(e.value))
	}
	return 0
}

// str reads an ASCII entry, inline when it fits in 4 bytes.
func (t tiffData) str(e entry) string {
	if e.typ != 2 || e.count == 0 || e.count > 256 {
		return ""
	}
	var raw []byte
	if e.count <= 4 {
		raw = e.value[:e.count]
	} else {
		off := t.entryU32(e)
		if off < 0 || off+int(e.count) > len(t.b) {
			return ""
		}
		raw = t.b[off : off+int(e.count)]
	}
	return strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
}

// rational reads a RATIONAL entry as numerator and denominator.
func (t tiffData) rational(e entry) (uint32, uint32, bool) {
	if e.typ != 5 || e.count < 1 {
		return 0, 0, false
	}
	off := t.entryU32(e)
	if off < 0 || off+8 > len(t.b) {
		return 0, 0, false
	}
	return t.bo.Uint32(t.b[off:]), t.bo.Uint32(t.b[off+4:]), true
}

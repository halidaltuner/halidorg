package photo

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// buildJPEG writes a minimal JPEG with an Exif APP1 segment in the given
// byte order: IFD0 (Make, Model, Exif pointer) and the Exif IFD.
func buildJPEG(bo binary.ByteOrder) []byte {
	var t bytes.Buffer
	if bo == binary.LittleEndian {
		t.WriteString("II")
	} else {
		t.WriteString("MM")
	}
	w16 := func(v uint16) { _ = binary.Write(&t, bo, v) }
	w32 := func(v uint32) { _ = binary.Write(&t, bo, v) }
	w16(42)
	w32(8)
	// Layout: IFD0 at 8 (3 entries), Exif IFD after it (5 entries), then data.
	ifd0 := 8
	exif := ifd0 + 2 + 3*12 + 4
	data := exif + 2 + 5*12 + 4
	strs := map[string]int{}
	var blob bytes.Buffer
	put := func(s string) int { o := data + blob.Len(); blob.WriteString(s + "\x00"); strs[s] = o; return o }
	rat := func(n, d uint32) int {
		o := data + blob.Len()
		_ = binary.Write(&blob, bo, n)
		_ = binary.Write(&blob, bo, d)
		return o
	}
	entry := func(tag, typ uint16, count, value uint32) { w16(tag); w16(typ); w32(count); w32(value) }
	short := func(tag, v uint16) { w16(tag); w16(3); w32(1); w16(v); w16(0) }
	makeOff, modelOff := put("FUJIFILM"), put("X-T5")
	lensOff, dateOff := put("XF23mmF1.4 R LM WR"), put("2026:09:14 18:42:07")
	expOff, fOff := rat(1, 250), rat(80, 10)
	w16(3)
	entry(tagMake, 2, 9, uint32(makeOff))
	entry(tagModel, 2, 5, uint32(modelOff))
	entry(tagExifIFD, 4, 1, uint32(exif))
	w32(0)
	w16(5)
	entry(tagExposureTime, 5, 1, uint32(expOff))
	entry(tagFNumber, 5, 1, uint32(fOff))
	short(tagISO, 160)
	entry(tagDateTimeOrig, 2, 20, uint32(dateOff))
	entry(tagLensModel, 2, uint32(len("XF23mmF1.4 R LM WR")+1), uint32(lensOff))
	w32(0)
	t.Write(blob.Bytes())
	seg := append([]byte("Exif\x00\x00"), t.Bytes()...)
	var j bytes.Buffer
	j.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	_ = binary.Write(&j, binary.BigEndian, uint16(len(seg)+2))
	j.Write(seg)
	j.Write([]byte{0xFF, 0xDA, 0, 2, 0xFF, 0xD9})
	return j.Bytes()
}

func TestReadEXIF(t *testing.T) {
	for _, bo := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		e := ReadEXIF(buildJPEG(bo))
		want := time.Date(2026, 9, 14, 18, 42, 7, 0, time.UTC)
		if e.Camera != "X-T5" || e.Lens != "XF23mmF1.4 R LM WR" || e.Exposure != "1/250 · f/8 · ISO 160" || e.TakenAt == nil || !e.TakenAt.Equal(want) {
			t.Errorf("%v: %+v", bo, e)
		}
	}
	// No EXIF, a PNG, truncated input: nothing, no panic.
	for _, b := range [][]byte{nil, {0xFF, 0xD8}, []byte("\x89PNG\r\n"), buildJPEG(binary.BigEndian)[:40]} {
		if e := ReadEXIF(b); e.Camera != "" || e.TakenAt != nil {
			t.Errorf("%q: %+v", b, e)
		}
	}
}

// Every prefix of a real EXIF block, and every byte flipped, reads without
// a panic.
func TestReadEXIFNeverPanics(t *testing.T) {
	full := buildJPEG(binary.LittleEndian)
	for i := range full {
		ReadEXIF(full[:i])
		b := append([]byte(nil), full...)
		b[i] ^= 0xFF
		ReadEXIF(b)
	}
}

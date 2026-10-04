// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
)

// PNG text keywords written by WritePNG.
const (
	KeyStage      = "mpg:stage"
	KeyConfigHash = "mpg:config-hash"
)

// Text is one PNG tEXt entry.
type Text struct {
	Key, Value string
}

// Meta is the provenance written into every render.
type Meta struct {
	// Stage names what the image shows, such as its stage file name
	// without the extension ("03-elevation"). Required.
	Stage string
	// ConfigHash is the resolved config's hash (config.Config.Hash).
	// Required.
	ConfigHash string
	// Extra entries follow the two required ones, in this order. Keys must
	// be unique and must not repeat KeyStage or KeyConfigHash.
	Extra []Text
}

// texts returns m's entries in file order, or an error if m is incomplete or
// an entry is not valid tEXt.
func (m Meta) texts() ([]Text, error) {
	if m.Stage == "" {
		return nil, errors.New("render: meta has no stage")
	}
	if m.ConfigHash == "" {
		return nil, errors.New("render: meta has no config hash")
	}
	all := append([]Text{{KeyStage, m.Stage}, {KeyConfigHash, m.ConfigHash}}, m.Extra...)
	seen := make(map[string]bool, len(all))
	for _, t := range all {
		if seen[t.Key] {
			return nil, fmt.Errorf("render: duplicate PNG text key %q", t.Key)
		}
		seen[t.Key] = true
		if err := checkText(t); err != nil {
			return nil, err
		}
	}
	return all, nil
}

// checkText reports whether t is a valid tEXt entry restricted to ASCII: a
// keyword of 1 to 79 printable characters without leading, trailing, or
// doubled spaces, and a value of printable characters and line feeds.
func checkText(t Text) error {
	k := t.Key
	if len(k) < 1 || len(k) > 79 {
		return fmt.Errorf("render: PNG text key %q must be 1 to 79 bytes", k)
	}
	for n := range len(k) {
		c := k[n]
		if c < 0x20 || c > 0x7e || (c == ' ' && (n == 0 || n == len(k)-1 || k[n-1] == ' ')) {
			return fmt.Errorf("render: PNG text key %q is not printable ASCII without stray spaces", k)
		}
	}
	for n := range len(t.Value) {
		if c := t.Value[n]; (c < 0x20 && c != '\n') || c > 0x7e {
			return fmt.Errorf("render: PNG text %q value has byte 0x%02x outside printable ASCII", k, c)
		}
	}
	return nil
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// WritePNG encodes img as a PNG with meta's entries as tEXt chunks right
// after the IHDR chunk, and writes it to w in a single Write.
func WritePNG(w io.Writer, img image.Image, meta Meta) error {
	texts, err := meta.texts()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("render: encode: %w", err)
	}
	b := buf.Bytes()
	// signature (8) + IHDR length, type, 13 data bytes, CRC (25)
	const ihdrEnd = 8 + 4 + 4 + 13 + 4
	if len(b) < ihdrEnd || !bytes.Equal(b[:8], pngSignature) || string(b[12:16]) != "IHDR" {
		return errors.New("render: encoder output does not start with IHDR")
	}
	out := make([]byte, 0, len(b)+64*len(texts))
	out = append(out, b[:ihdrEnd]...)
	for _, t := range texts {
		data := make([]byte, 0, len(t.Key)+1+len(t.Value))
		data = append(append(append(data, t.Key...), 0), t.Value...)
		out = appendChunk(out, "tEXt", data)
	}
	out = append(out, b[ihdrEnd:]...)
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("render: write: %w", err)
	}
	return nil
}

// appendChunk appends a PNG chunk: length, type, data, and the CRC-32 of
// type and data.
func appendChunk(b []byte, typ string, data []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(data)))
	start := len(b)
	b = append(append(b, typ...), data...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b[start:]))
}

// WritePNGFile writes img with meta to path, creating or truncating it.
func WritePNGFile(path string, img image.Image, meta Meta) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	if err := WritePNG(f, img, meta); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	return nil
}

// ReadPNGText returns the tEXt entries of the PNG read from r, in file order.
// It checks the signature and every chunk's CRC and stops at IEND.
func ReadPNGText(r io.Reader) ([]Text, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("render: read: %w", err)
	}
	if !bytes.HasPrefix(b, pngSignature) {
		return nil, errors.New("render: not a PNG file")
	}
	var texts []Text
	for p := len(pngSignature); ; {
		if len(b)-p < 12 {
			return nil, errors.New("render: PNG truncated before IEND")
		}
		n := binary.BigEndian.Uint32(b[p:])
		if uint64(n) > uint64(len(b)-p-12) {
			return nil, errors.New("render: PNG chunk runs past the end of the file")
		}
		end := p + 8 + int(n)
		typ, data := string(b[p+4:p+8]), b[p+8:end]
		if crc32.ChecksumIEEE(b[p+4:end]) != binary.BigEndian.Uint32(b[end:]) {
			return nil, fmt.Errorf("render: PNG chunk %q has a bad CRC", typ)
		}
		switch typ {
		case "tEXt":
			key, value, ok := bytes.Cut(data, []byte{0})
			if !ok {
				return nil, errors.New("render: tEXt chunk has no keyword separator")
			}
			texts = append(texts, Text{Key: string(key), Value: string(value)})
		case "IEND":
			return texts, nil
		}
		p = end + 4
	}
}

// ReadMeta returns the provenance of a PNG written by WritePNG: the stage and
// config hash, with every other tEXt entry in Extra. It fails if either
// required entry is missing.
func ReadMeta(r io.Reader) (Meta, error) {
	texts, err := ReadPNGText(r)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	for _, t := range texts {
		switch t.Key {
		case KeyStage:
			m.Stage = t.Value
		case KeyConfigHash:
			m.ConfigHash = t.Value
		default:
			m.Extra = append(m.Extra, t)
		}
	}
	if m.Stage == "" || m.ConfigHash == "" {
		return Meta{}, errors.New("render: PNG has no mpg:stage or mpg:config-hash text")
	}
	return m, nil
}

// PixelHash returns the hex SHA-256 of img's pixels: the width and height as
// big-endian uint32, then every pixel's 8-bit R, G, B, A in row order, top to
// bottom and left to right. Pixels are taken in color.RGBA (alpha
// premultiplied), which equals straight alpha for the opaque images renders
// produce, so an image and its PNG round trip hash equally whatever Go image
// type the decoder returns.
func PixelHash(img image.Image) string {
	b := img.Bounds()
	h := sha256.New()
	var head [8]byte
	binary.BigEndian.PutUint32(head[0:], uint32(b.Dx()))
	binary.BigEndian.PutUint32(head[4:], uint32(b.Dy()))
	h.Write(head[:])
	row := make([]byte, 4*b.Dx())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if m, ok := img.(*image.RGBA); ok {
			o := m.PixOffset(b.Min.X, y)
			h.Write(m.Pix[o : o+len(row)])
			continue
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			k := 4 * (x - b.Min.X)
			row[k], row[k+1], row[k+2], row[k+3] = c.R, c.G, c.B, c.A
		}
		h.Write(row)
	}
	return hex.EncodeToString(h.Sum(nil))
}

package ride_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/ride"
)

// Un PNG diminuto que declara 50 000 × 50 000 píxeles se rechaza antes de
// decodificarlo (riesgo no confirmado de la auditoría de v0.1.0: la
// decodificación habría reservado unos 10 GB).
func TestLogoConDimensionesExcesivas(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// IHDR: 8 bytes de firma, 4 de longitud y 4 de tipo; luego ancho y alto.
	binary.BigEndian.PutUint32(b[16:], 50000)
	binary.BigEndian.PutUint32(b[20:], 50000)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	doc := facturaDePrueba(t, 1)
	if _, err := ride.Generar(doc, &ride.Opciones{Logo: b}); !errors.Is(err, ride.ErrLogo) {
		t.Errorf("err = %v", err)
	}
}

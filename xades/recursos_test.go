package xades

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// Segunda auditoría, N-06: un comprobante firmado con la marca de orden de
// bytes (BOM) de UTF-8 delante es el mismo documento.
func TestBOMUTF8(t *testing.T) {
	firmado := firmar(t, SHA256)
	conBOM := append([]byte{0xEF, 0xBB, 0xBF}, firmado...)
	if _, err := Verificar(conBOM, nil); err != nil {
		t.Errorf("Verificar con BOM: %v", err)
	}
	a, errA := Huella(firmado)
	b, errB := Huella(conBOM)
	if errA != nil || errB != nil || a != b {
		t.Errorf("la huella cambia con el BOM: %v %v", errA, errB)
	}
	// Firmar un documento con BOM produce una firma válida, sin BOM.
	out, err := Firmar(append([]byte{0xEF, 0xBB, 0xBF}, facturaMinima...), mat(t).Certificado, &Opciones{Momento: momento})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("el documento firmado conserva el BOM")
	}
	if _, err := Verificar(out, nil); err != nil {
		t.Errorf("Verificar: %v", err)
	}
	// Solo se admite un BOM, y solo al principio.
	if _, err := Verificar(append([]byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF}, firmado...), nil); err == nil {
		t.Error("se aceptaron dos BOM")
	}
	// Dentro del contenido es un carácter más, cubierto por la firma.
	alterado := bytes.Replace(firmado, []byte("<ruc>"), append([]byte("<ruc>"), 0xEF, 0xBB, 0xBF), 1)
	if _, err := Verificar(alterado, nil); !errors.Is(err, ErrDigest) {
		t.Errorf("un U+FEFF añadido al contenido debe invalidar la firma: %v", err)
	}
}

// Segunda auditoría, N-04: cada referencia obliga a canonicalizar y resumir
// su destino. Sin límite, una firma con miles de referencias al documento
// entero multiplica el coste de verificar por el número de referencias.
func TestDemasiadasReferencias(t *testing.T) {
	// (Medido sin el límite: 3000 referencias a un documento de 2 MB ocupan
	// 91 s; el máximo que cabe en MaxBytes, horas.)
	grande := strings.Replace(facturaMinima, "<vacio/>", "<vacio>"+strings.Repeat("<a>relleno</a>", 2_000)+"</vacio>", 1)
	firmado, err := Firmar([]byte(grande), mat(t).Certificado, &Opciones{Momento: momento})
	if err != nil {
		t.Fatal(err)
	}
	h, err := Huella(firmado)
	if err != nil {
		t.Fatal(err)
	}
	// Cada referencia añadida es correcta: apunta al documento entero con
	// la transformación enveloped y declara su digest verdadero.
	ref := `<ds:Reference URI=""><ds:Transforms><ds:Transform Algorithm="` + AlgEnveloped + `"></ds:Transform></ds:Transforms>` +
		`<ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"></ds:DigestMethod><ds:DigestValue>` +
		base64.StdEncoding.EncodeToString(h[:]) + `</ds:DigestValue></ds:Reference>`
	if !bytes.Contains(firmado, []byte("</ds:SignedInfo>")) {
		t.Fatal("la prueba espera el prefijo ds")
	}
	con := func(n int) []byte {
		return bytes.Replace(firmado, []byte("</ds:SignedInfo>"), []byte(strings.Repeat(ref, n)+"</ds:SignedInfo>"), 1)
	}
	if _, err = Verificar(con(3000), nil); !errors.Is(err, ErrEstructura) {
		t.Errorf("3000 referencias: err=%v; se esperaba ErrEstructura", err)
	}
	// Dentro del límite se procesan todas, y la firma, que no las cubría,
	// resulta inválida.
	if _, err := Verificar(con(MaxReferencias-3), nil); !errors.Is(err, ErrFirmaInvalida) {
		t.Errorf("con %d referencias: %v", MaxReferencias, err)
	}
	if _, err := Verificar(con(MaxReferencias-2), nil); !errors.Is(err, ErrEstructura) {
		t.Errorf("con %d referencias: %v", MaxReferencias+1, err)
	}
}

func TestDemasiadosCertificados(t *testing.T) {
	firmado := firmar(t, SHA256)
	i := bytes.Index(firmado, []byte("<ds:X509Certificate>"))
	j := bytes.Index(firmado, []byte("</ds:X509Certificate>")) + len("</ds:X509Certificate>")
	if i < 0 || j < i {
		t.Fatal("la prueba espera el prefijo ds")
	}
	cert := string(firmado[i:j])
	muchos := bytes.Replace(firmado, []byte(cert), []byte(strings.Repeat(cert, MaxCertificados+1)), 1)
	if _, err := Verificar(muchos, nil); !errors.Is(err, ErrEstructura) {
		t.Errorf("con %d certificados: %v", MaxCertificados+1, err)
	}
}

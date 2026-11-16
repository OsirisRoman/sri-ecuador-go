package xades

import (
	"bytes"
	"testing"
)

// La huella identifica el contenido del comprobante: no cambia con la
// serialización ni con otra firma del mismo contenido, y cambia con
// cualquier alteración (AUD-002).
func TestHuella(t *testing.T) {
	a := firmar(t, SHA256)
	b := firmar(t, SHA1) // otra firma (otros Id, otro algoritmo) del mismo contenido
	ha, err := Huella(a)
	if err != nil {
		t.Fatal(err)
	}
	if hb, _ := Huella(b); hb != ha {
		t.Error("dos firmas del mismo contenido dan huellas distintas")
	}
	sinFirma := a[:bytes.Index(a, []byte("<ds:Signature"))]
	sinFirma = append(append([]byte{}, sinFirma...), a[bytes.LastIndex(a, []byte("</ds:Signature>"))+len("</ds:Signature>"):]...)
	if h, _ := Huella(sinFirma); h != ha {
		t.Error("el documento sin firma debe tener la misma huella")
	}
	otraSerializacion := bytes.Replace(a, []byte(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"), []byte("<?xml version='1.0'?>\n\n"), 1)
	if h, _ := Huella(otraSerializacion); h != ha || bytes.Equal(otraSerializacion, a) {
		t.Error("la declaración XML no forma parte del contenido")
	}
	alterado := bytes.Replace(a, []byte("<ruc>1790000000001</ruc>"), []byte("<ruc>1790000000002</ruc>"), 1)
	if h, _ := Huella(alterado); h == ha || bytes.Equal(alterado, a) {
		t.Error("una alteración del contenido no cambió la huella")
	}
	if _, err := Huella([]byte("<a>")); err == nil {
		t.Error("un documento mal formado debe dar error")
	}
}

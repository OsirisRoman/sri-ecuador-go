package xmldom

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
)

// AUD-008: atributos repetidos hacen que el documento no esté bien formado.
func TestAtributosRepetidos(t *testing.T) {
	for _, doc := range []string{
		`<a x="1" x="2"/>`,
		`<factura id="comprobante" version="1.1.0" version="2.1.0"/>`,
		`<a xmlns:p="urn:u" xmlns:q="urn:u" p:x="1" q:x="2"/>`,
		`<a xmlns:p="urn:u" xmlns:p="urn:v"/>`,
	} {
		if _, err := Analizar([]byte(doc)); !errors.Is(err, ErrMalFormado) {
			t.Errorf("%s: err = %v", doc, err)
		}
	}
	if _, err := Analizar([]byte(`<a xmlns:p="urn:u" xmlns:q="urn:v" p:x="1" q:x="2" x="3"/>`)); err != nil {
		t.Errorf("mismo nombre local en espacios distintos es válido: %v", err)
	}
}

// AUD-008: normalización de valores de atributo (XML 1.0, 3.3.3).
func TestNormalizacionDeAtributos(t *testing.T) {
	for _, c := range []struct{ crudo, valor string }{
		{"x\ny", "x y"},
		{"x\r\ny", "x y"},
		{"x\ry", "x y"},
		{"x\ty", "x y"},
		{"x&#xA;y", "x\ny"},
		{"x&#9;y&#xD;", "x\ty\r"},
		{"&lt;&gt;&amp;&quot;&apos;&#x41;&#66;", `<>&"'AB`},
		{"a > b\n", "a > b "},
	} {
		doc := "<a\n  b=\"" + c.crudo + "\"\n  c='\"\n'/>"
		d, err := Analizar([]byte(doc))
		if err != nil {
			t.Errorf("%q: %v", c.crudo, err)
			continue
		}
		if v, _ := d.Raiz.Attr("b"); v != c.valor {
			t.Errorf("%q: valor %q, se esperaba %q", c.crudo, v, c.valor)
		}
		if v, _ := d.Raiz.Attr("c"); v != `" ` {
			t.Errorf("c = %q", v)
		}
	}
}

// La forma canónica de un atributo con un salto de línea literal es la de
// libxml2: un espacio, no &#xA; (la diferencia que encontró la auditoría).
func TestCanonicaDeAtributoConSaltoLiteral(t *testing.T) {
	d, err := Analizar([]byte("<a b=\"x\ny\" c=\"x&#xA;y\"/>"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(Canonicalizar(d.Raiz, nil)), `<a b="x y" c="x&#xA;y"></a>`; got != want {
		t.Errorf("C14N = %s, se esperaba %s", got, want)
	}
}

// Propiedad: el valor normalizado es el que da encoding/xml al mismo texto
// con los blancos literales ya sustituidos por espacios.
func FuzzNormalizacionDeAtributos(f *testing.F) {
	for _, s := range []string{"x\ny", "a&#xA;b\t", "&lt;\r\n&amp;", "", "\r"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, crudo string) {
		if strings.ContainsAny(crudo, `"<`) {
			return
		}
		d, err := Analizar([]byte(`<a b="` + crudo + `"/>`))
		if err != nil {
			return
		}
		got, _ := d.Raiz.Attr("b")
		esp := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(crudo)
		var ref struct {
			B string `xml:"b,attr"`
		}
		if err := xml.Unmarshal([]byte(`<a b="`+esp+`"/>`), &ref); err != nil {
			t.Skip()
		}
		if got != ref.B {
			t.Fatalf("crudo %q: obtuvo %q, referencia %q", crudo, got, ref.B)
		}
	})
}

// Propiedad: la forma canónica es un punto fijo, C14N(analizar(C14N(x))) =
// C14N(x). Durante la auditoría se ejecutó 10 millones de veces sin fallos.
func FuzzC14NIdempotente(f *testing.F) {
	for _, s := range []string{
		`<a xmlns="u" b="1"><c xmlns="">t&#xD;</c></a>`,
		`<p:a xmlns:p="u" xmlns:q="v" q:z="1" y="2"><b xml:lang="es"/></p:a>`,
		`<a>x<![CDATA[<y>]]>&amp;<?pi d?></a>`,
		"<a b=\"x\ny\" c='&#xA;'/>",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		d, err := Analizar(in)
		if err != nil {
			return
		}
		c1 := Canonicalizar(d.Raiz, nil)
		d2, err := Analizar(c1)
		if err != nil {
			t.Fatalf("la forma canónica no se vuelve a analizar: %v\n%q\n%q", err, in, c1)
		}
		if c2 := Canonicalizar(d2.Raiz, nil); !bytes.Equal(c1, c2) {
			t.Fatalf("no idempotente\nin=%q\nc1=%q\nc2=%q", in, c1, c2)
		}
	})
}

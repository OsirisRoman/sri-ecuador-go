package xmldom

import (
	"errors"
	"testing"
)

func c14n(t *testing.T, in string) string {
	t.Helper()
	d, err := Analizar([]byte(in))
	if err != nil {
		t.Fatalf("Analizar: %v", err)
	}
	return string(Canonicalizar(d.Raiz, nil))
}

func TestCanonicalizar(t *testing.T) {
	casos := []struct{ nombre, in, want string }{
		{"vacíos y comentarios", `<?xml version="1.0"?><a><!-- c --><b/><c></c></a>`, `<a><b></b><c></c></a>`},
		// Adaptado de C14N 1.0 §3.3 (sin la parte que requiere DTD).
		{"atributos y ns ordenados", `<e3 name = "elem3" id="elem3" /><!-- x -->`, `<e3 id="elem3" name="elem3"></e3>`},
		{"orden de atributos con ns",
			`<e5 a:attr="out" b:attr="sorted" attr2="all" attr="I'm" xmlns:b="http://www.ietf.org" xmlns:a="http://www.w3.org" xmlns="http://example.org"/>`,
			`<e5 xmlns="http://example.org" xmlns:a="http://www.w3.org" xmlns:b="http://www.ietf.org" attr="I'm" attr2="all" b:attr="sorted" a:attr="out"></e5>`},
		{"ns redundante", `<a xmlns:x="u"><x:b xmlns:x="u"><c xmlns:x="v"/></x:b></a>`, `<a xmlns:x="u"><x:b><c xmlns:x="v"></c></x:b></a>`},
		{"xmlns vacío", `<a xmlns="u"><b xmlns=""><c/></b></a>`, `<a xmlns="u"><b xmlns=""><c></c></b></a>`},
		{"xmlns vacío superfluo", `<a xmlns=""><b xmlns=""/></a>`, `<a><b></b></a>`},
		// C14N 1.0 §3.4.
		{"escapes de texto", "<t>First line&#x0d;&#10;Second line &lt; &gt; &amp; \"q\"</t>", "<t>First line&#xD;\nSecond line &lt; &gt; &amp; \"q\"</t>"},
		{"escapes de atributo", `<t a="&quot;&lt;&amp;&#x9;&#xA;&#xD;'>"/>`, `<t a="&quot;&lt;&amp;&#x9;&#xA;&#xD;'>"></t>`},
		{"CDATA", `<t>a<![CDATA[<b> & ]]>c</t>`, `<t>a&lt;b&gt; &amp; c</t>`},
		{"espacios preservados", "<a>\n  <b> x </b>\n</a>", "<a>\n  <b> x </b>\n</a>"},
		{"CRLF normalizado", "<a>x\r\ny</a>", "<a>x\ny</a>"},
		{"UTF-8", `<a n="ñ">Áé€</a>`, `<a n="ñ">Áé€</a>`},
		{"instrucción de procesamiento", `<a><?pi dato ?><?vacia?></a>`, `<a><?pi dato ?><?vacia?></a>`},
	}
	for _, c := range casos {
		if got := c14n(t, c.in); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.nombre, got, c.want)
		}
	}
}

func TestCanonicalizarSubarbol(t *testing.T) {
	d, err := Analizar([]byte(`<r xmlns="d" xmlns:p="q" xml:lang="es" id="1"><p:a><b at="1"/></p:a><z/></r>`))
	if err != nil {
		t.Fatal(err)
	}
	a := d.Raiz.Elementos()[0]
	// El vértice hereda los espacios de nombres y los atributos xml:*.
	if got, want := string(Canonicalizar(a, nil)), `<p:a xmlns="d" xmlns:p="q" xml:lang="es"><b at="1"></b></p:a>`; got != want {
		t.Errorf("subárbol:\n got  %s\n want %s", got, want)
	}
	// Exclusión (transformación enveloped).
	if got, want := string(Canonicalizar(d.Raiz, a)), `<r xmlns="d" xmlns:p="q" id="1" xml:lang="es"><z></z></r>`; got != want {
		t.Errorf("exclusión:\n got  %s\n want %s", got, want)
	}
}

func TestRechazos(t *testing.T) {
	casos := map[string]error{
		`<!DOCTYPE a [<!ENTITY x "y">]><a>&x;</a>`: ErrDoctype,
		`<a><b></a>`:           ErrMalFormado,
		`<a/><b/>`:             ErrMalFormado,
		`<a>`:                  ErrMalFormado,
		`<p:a/>`:               ErrPrefijoNoNS,
		`<a p:x="1"/>`:         ErrPrefijoNoNS,
		`<a>&desconocida;</a>`: ErrMalFormado,
		``:                     ErrMalFormado,
		`<?xml version="1.0" encoding="ISO-8859-1"?><a/>`: ErrMalFormado,
	}
	for in, want := range casos {
		if _, err := Analizar([]byte(in)); !errors.Is(err, want) {
			t.Errorf("Analizar(%q) = %v, quiero %v", in, err, want)
		}
	}
	profundo := ""
	for i := 0; i < MaxProfundidad+1; i++ {
		profundo += "<a>"
	}
	if _, err := Analizar([]byte(profundo)); !errors.Is(err, ErrDemasiado) {
		t.Errorf("profundidad: %v", err)
	}
}

func TestBuscarPorID(t *testing.T) {
	d, _ := Analizar([]byte(`<a id="x"><b Id="y"/><c ID="y"/><d p:id="x" xmlns:p="u"/></a>`))
	if n := len(d.Raiz.BuscarPorID("x")); n != 1 {
		t.Errorf("x: %d", n)
	}
	if n := len(d.Raiz.BuscarPorID("y")); n != 2 {
		t.Errorf("y: %d", n)
	}
}

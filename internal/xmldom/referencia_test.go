package xmldom

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
)

func canonicalizarReferencia(e *Elemento, excluir *Elemento) []byte {
	var buf bytes.Buffer
	// Espacios de nombres en ámbito para el vértice.
	enAmbito := map[string]string{}
	var cadena []*Elemento
	for x := e; x != nil; x = x.Padre {
		cadena = append(cadena, x)
	}
	for i := len(cadena) - 1; i >= 0; i-- {
		for p, u := range cadena[i].NS {
			enAmbito[p] = u
		}
	}
	// Atributos xml:* heredados (xml:lang, xml:space, ...).
	var heredados []Atributo
	for i := 1; i < len(cadena); i++ { // del padre hacia la raíz
		for _, a := range cadena[i].Attrs {
			if a.Prefijo != "xml" {
				continue
			}
			ya := false
			for _, h := range heredados {
				if h.Local == a.Local {
					ya = true
				}
			}
			for _, propio := range e.Attrs {
				if propio.Prefijo == "xml" && propio.Local == a.Local {
					ya = true
				}
			}
			if !ya {
				heredados = append(heredados, a)
			}
		}
	}
	canonReferencia(&buf, e, excluir, enAmbito, map[string]string{}, heredados)
	return buf.Bytes()
}

// canonReferencia es el algoritmo anterior de canonicalización, que en cada
// elemento recorría todos los espacios de nombres en ámbito (coste
// cuadrático). Se conserva como oráculo del algoritmo proporcional.
//
// canonReferencia escribe e. enAmbito son los espacios visibles en e (incluidos los
// propios); emitidos, los que ya quedaron escritos por un ancestro dentro de
// la salida.
func canonReferencia(buf *bytes.Buffer, e, excluir *Elemento, enAmbito, emitidos map[string]string, extra []Atributo) {
	nombre := e.Local
	if e.Prefijo != "" {
		nombre = e.Prefijo + ":" + e.Local
	}
	buf.WriteByte('<')
	buf.WriteString(nombre)

	// Declaraciones de espacios de nombres: todo lo que está en ámbito y no
	// fue emitido con el mismo valor por el ancestro de salida más cercano.
	misEmitidos := emitidos
	var prefijos []string
	for p, u := range enAmbito {
		if p == "xml" {
			continue
		}
		if prev, ok := emitidos[p]; ok && prev == u {
			continue
		}
		if p == "" && u == "" {
			// xmlns="" solo se escribe para anular un valor por defecto
			// no vacío ya emitido.
			if prev, ok := emitidos[""]; !ok || prev == "" {
				continue
			}
		}
		prefijos = append(prefijos, p)
	}
	if len(prefijos) > 0 {
		sort.Strings(prefijos) // "" (xmlns por defecto) ordena primero
		misEmitidos = make(map[string]string, len(emitidos)+len(prefijos))
		for p, u := range emitidos {
			misEmitidos[p] = u
		}
		for _, p := range prefijos {
			u := enAmbito[p]
			misEmitidos[p] = u
			if p == "" {
				buf.WriteString(` xmlns="`)
			} else {
				buf.WriteString(" xmlns:" + p + `="`)
			}
			escAttr(buf, u)
			buf.WriteByte('"')
		}
	}

	// Atributos ordenados por (URI del espacio de nombres, nombre local);
	// los que no tienen prefijo no tienen espacio y van primero.
	type attrOrd struct {
		uri string
		a   Atributo
	}
	attrs := make([]attrOrd, 0, len(e.Attrs)+len(extra))
	for _, a := range append(append([]Atributo(nil), e.Attrs...), extra...) {
		uri := ""
		if a.Prefijo == "xml" {
			uri = XMLNamespace
		} else if a.Prefijo != "" {
			uri = enAmbito[a.Prefijo]
		}
		attrs = append(attrs, attrOrd{uri, a})
	}
	sort.SliceStable(attrs, func(i, j int) bool {
		if attrs[i].uri != attrs[j].uri {
			return attrs[i].uri < attrs[j].uri
		}
		return attrs[i].a.Local < attrs[j].a.Local
	})
	for _, x := range attrs {
		buf.WriteByte(' ')
		if x.a.Prefijo != "" {
			buf.WriteString(x.a.Prefijo + ":")
		}
		buf.WriteString(x.a.Local)
		buf.WriteString(`="`)
		escAttr(buf, x.a.Valor)
		buf.WriteByte('"')
	}
	buf.WriteByte('>')

	for _, h := range e.Hijos {
		switch n := h.(type) {
		case Texto:
			escTexto(buf, string(n))
		case InstruccionProc:
			buf.WriteString("<?" + n.Destino)
			if n.Datos != "" {
				buf.WriteString(" " + n.Datos)
			}
			buf.WriteString("?>")
		case *Elemento:
			if n == excluir {
				continue
			}
			ambitoHijo := enAmbito
			if len(n.NS) > 0 {
				ambitoHijo = make(map[string]string, len(enAmbito)+len(n.NS))
				for p, u := range enAmbito {
					ambitoHijo[p] = u
				}
				for p, u := range n.NS {
					ambitoHijo[p] = u
				}
			}
			canonReferencia(buf, n, excluir, ambitoHijo, misEmitidos, nil)
		}
	}
	buf.WriteString("</" + nombre + ">")
}

// documentoAleatorio genera un documento con espacios de nombres
// declarados, redeclarados y anulados a distintas profundidades, atributos
// con y sin prefijo, xml:lang, texto y secciones CDATA.
func documentoAleatorio(r *rand.Rand) string {
	prefijos := []string{"", "a", "b", "c"}
	uris := []string{"urn:1", "urn:2", "urn:3", ""}
	var sb strings.Builder
	var elemento func(prof int, enAmbito map[string]string)
	elemento = func(prof int, enAmbito map[string]string) {
		ambito := map[string]string{}
		for k, v := range enAmbito {
			ambito[k] = v
		}
		decl := ""
		for i := r.IntN(3); i > 0; i-- {
			p, u := prefijos[r.IntN(len(prefijos))], uris[r.IntN(len(uris))]
			if p != "" && u == "" {
				continue // xmlns:p="" no es válido
			}
			if _, ya := ambito["decl:"+p]; ya {
				continue
			}
			ambito["decl:"+p] = "1"
			ambito[p] = u
			if p == "" {
				decl += fmt.Sprintf(` xmlns="%s"`, u)
			} else {
				decl += fmt.Sprintf(` xmlns:%s="%s"`, p, u)
			}
		}
		hijo := map[string]string{}
		var ligados []string
		for k, v := range ambito {
			if !strings.HasPrefix(k, "decl:") {
				hijo[k] = v
				if k != "" && v != "" {
					ligados = append(ligados, k)
				}
			}
		}
		sort.Strings(ligados)
		nombre := "e"
		if len(ligados) > 0 && r.IntN(2) == 0 {
			nombre = ligados[r.IntN(len(ligados))] + ":e"
		}
		sb.WriteString("<" + nombre + decl)
		if r.IntN(3) == 0 {
			sb.WriteString(` z="1" id="x"`)
		}
		if len(ligados) > 0 && r.IntN(3) == 0 {
			sb.WriteString(" " + ligados[r.IntN(len(ligados))] + `:at="v"`)
		}
		if r.IntN(6) == 0 {
			sb.WriteString(` xml:lang="es"`)
		}
		sb.WriteString(">")
		for i := r.IntN(7); i > 0 && prof < 5; i-- {
			switch r.IntN(4) {
			case 0:
				sb.WriteString("t&amp;x")
			case 1:
				sb.WriteString("<![CDATA[<c>]]>")
			default:
				elemento(prof+1, hijo)
			}
		}
		sb.WriteString("</" + nombre + ">")
	}
	elemento(0, map[string]string{})
	return sb.String()
}

// El algoritmo proporcional produce exactamente la misma forma canónica
// que el anterior, para el documento entero y para cada subárbol (que
// hereda espacios de nombres y atributos xml:* de sus ancestros), con y sin
// un elemento excluido.
func TestCanonicalizacionIgualQueLaReferencia(t *testing.T) {
	r := rand.New(rand.NewPCG(4, 2026))
	subarboles := 0
	for caso := 0; caso < 4000; caso++ {
		doc := documentoAleatorio(r)
		d, err := Analizar([]byte(doc))
		if err != nil {
			t.Fatalf("caso %d: %v\n%s", caso, err, doc)
		}
		var todos []*Elemento
		d.Raiz.Recorrer(func(e *Elemento) bool { todos = append(todos, e); return true })
		for _, e := range todos {
			var excluir *Elemento
			if r.IntN(2) == 0 {
				excluir = todos[r.IntN(len(todos))]
				if excluir == e {
					excluir = nil
				}
			}
			a, b := Canonicalizar(e, excluir), canonicalizarReferencia(e, excluir)
			if !bytes.Equal(a, b) {
				t.Fatalf("caso %d: formas canónicas distintas\ndocumento:  %s\nnueva:      %s\nreferencia: %s", caso, doc, a, b)
			}
			subarboles++
		}
		// Canonicalizar no deja rastro: una segunda pasada da lo mismo.
		if a, b := Canonicalizar(d.Raiz, nil), Canonicalizar(d.Raiz, nil); !bytes.Equal(a, b) {
			t.Fatalf("caso %d: dos pasadas distintas", caso)
		}
	}
	t.Logf("%d subárboles comparados", subarboles)
}

// FuzzCanonicalizacionIgualQueLaReferencia hace la misma comparación sobre
// documentos arbitrarios.
func FuzzCanonicalizacionIgualQueLaReferencia(f *testing.F) {
	for _, s := range []string{
		`<a xmlns="u" xmlns:p="v"><p:b xmlns=""><c xmlns="u"/></p:b></a>`,
		`<a xmlns:p="1"><b xmlns:p="2"><c xmlns:p="1" p:x="1"/></b><d p:y="2"/></a>`,
		`<a xml:lang="es"><b xml:space="preserve"><c/></b>x<![CDATA[y]]><!--z-->w</a>`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := Analizar([]byte(s))
		if err != nil {
			return
		}
		d.Raiz.Recorrer(func(e *Elemento) bool {
			if a, b := Canonicalizar(e, nil), canonicalizarReferencia(e, nil); !bytes.Equal(a, b) {
				t.Fatalf("formas canónicas distintas\nnueva:      %s\nreferencia: %s", a, b)
			}
			return true
		})
	})
}

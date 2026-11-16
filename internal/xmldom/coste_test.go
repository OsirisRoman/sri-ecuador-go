package xmldom

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Segunda auditoría, N-04: el coste de analizar y canonicalizar debe ser
// proporcional al tamaño del documento, también para documentos hostiles
// dentro del límite de MaxBytes. Cada caso ocupa unos pocos megabytes y, con
// un algoritmo cuadrático, minutos u horas de CPU.
func TestCosteProporcional(t *testing.T) {
	declaraciones := func(n int) string {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&sb, ` xmlns:n%d="u"`, i)
		}
		return sb.String()
	}
	casos := map[string]string{
		// Muchos espacios de nombres en ámbito y muchos descendientes.
		"espacios en ámbito × hijos": "<a" + declaraciones(20000) + ">" + strings.Repeat("<b/>", 300000) + "</a>",
		// Lo mismo, y cada hijo declara además el suyo.
		"espacios en ámbito × hijos que declaran": "<a" + declaraciones(20000) + ">" + strings.Repeat(`<b xmlns:z="v"/>`, 300000) + "</a>",
		// Cada hijo vuelve a declarar un prefijo ya en ámbito con otro valor.
		"redeclaraciones": "<a" + declaraciones(20000) + ">" + strings.Repeat(`<b xmlns:n7="w"><c/></b>`, 200000) + "</a>",
		// Texto partido en muchos fragmentos por secciones CDATA y comentarios.
		"fragmentos CDATA":       "<a>" + strings.Repeat("x<![CDATA[y]]>", 500000) + "</a>",
		"fragmentos comentarios": "<a>" + strings.Repeat("x<!---->", 900000) + "</a>",
		// Muchos atributos en un elemento.
		"atributos": "<a" + strings.Replace(declaraciones(100000), "xmlns:", "", -1) + "/>",
	}
	for nombre, doc := range casos {
		if len(doc) > MaxBytes {
			t.Fatalf("%s: el caso (%d bytes) supera MaxBytes", nombre, len(doc))
		}
		ini := time.Now()
		d, err := Analizar([]byte(doc))
		if err != nil {
			t.Errorf("%s: %v", nombre, err)
			continue
		}
		salida := Canonicalizar(d.Raiz, nil)
		if dur := time.Since(ini); dur > 10*time.Second {
			t.Errorf("%s: %d kB tardaron %v", nombre, len(doc)/1024, dur)
		} else {
			t.Logf("%s: %d kB → %d kB canónicos en %v", nombre, len(doc)/1024, len(salida)/1024, dur)
		}
	}
}

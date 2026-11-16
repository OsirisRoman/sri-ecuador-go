package ride_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/ride"
)

// AUD-013: el RIDE de un comprobante sin autorización conocida se marca
// como vista previa; el de uno autorizado, no.
func TestVistaPreviaMarcada(t *testing.T) {
	doc := facturaDePrueba(t, 1)
	envuelto := []byte(`<?xml version="1.0" encoding="UTF-8"?><autorizacion><estado>AUTORIZADO</estado><numeroAutorizacion>1234567890</numeroAutorizacion><fechaAutorizacion>2026-11-14T23:56:59-05:00</fechaAutorizacion>` +
		`<ambiente>PRUEBAS</ambiente><comprobante><![CDATA[` + string(doc) + `]]></comprobante></autorizacion>`)
	for _, c := range []struct {
		nombre string
		pdf    []byte
		marca  bool
	}{
		{"sin envoltura", must(ride.Generar(doc, nil)), true},
		{"sin envoltura con número pero sin fecha", must(ride.Generar(doc, &ride.Opciones{NumeroAutorizacion: "1234567890"})), true},
		{"sin envoltura con fecha de autorización", must(ride.Generar(doc, &ride.Opciones{FechaAutorizacion: "15/11/2026 10:00:00"})), false},
		{"envuelto en AUTORIZADO", must(ride.Generar(envuelto, nil)), false},
	} {
		texto, _ := contenido(t, c.pdf)
		tiene := strings.Contains(texto, "(SIN AUTORIZACI") && strings.Contains(texto, "(VISTA PREVIA: SIN VALIDEZ TRIBUTARIA)")
		if tiene != c.marca {
			t.Errorf("%s: marca de vista previa = %v, se esperaba %v", c.nombre, tiene, c.marca)
		}
		if titulo := strings.Contains(string(c.pdf), "/Title <FEFF"+utf16("VISTA PREVIA SIN VALIDEZ")); titulo != c.marca {
			t.Errorf("%s: título del PDF marcado = %v", c.nombre, titulo)
		}
	}
}

// utf16 codifica s como en los metadatos del PDF (UTF-16BE en hexadecimal).
func utf16(s string) string {
	var sb strings.Builder
	for _, r := range s {
		fmt.Fprintf(&sb, "%04X", r)
	}
	return sb.String()
}

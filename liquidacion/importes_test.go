package liquidacion_test

import (
	"testing"
)

// AUD-001 y AUD-003: la liquidación comparte las reglas de los detalles de
// la factura.
func TestBaseYValorDelIVAPorLinea(t *testing.T) {
	l := base(t, hoy)
	imp := &l.Detalles[1].Impuestos[0] // 20,00 al 15 %
	imp.BaseImponible = d("2.00")
	if hs := validar(l); !hs.Tiene("iva.base") {
		t.Errorf("falta iva.base: %v", hs)
	}
	l = base(t, hoy)
	l.Detalles[1].Impuestos[0].Valor = d("3.01") // correcto: 3,00
	if hs := validar(l); !hs.Tiene("iva.valor") {
		t.Errorf("falta iva.valor: %v", hs)
	}
}

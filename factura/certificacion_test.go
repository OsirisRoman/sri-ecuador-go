package factura_test

// Los casos de redondeo de la ronda de certificación de 0.2.0 en el ambiente
// de pruebas del SRI (docs/pruebas-sri.md), con su resultado en el SDK.

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
)

// T3 y T4: redondeos legítimos que el SRI autorizó; el SDK los acepta.
// T5: un centavo de más en una línea que el SRI autorizó; el SDK lo rechaza
// por decisión (D17).
func TestCasosDeRedondeoDelSRI(t *testing.T) {
	casos := []struct{ nombre, linea, total string }{
		{"T3: suma de líneas (0,02 × 10 = 0,20; acumulado 0,15)", "0.02", "0.20"},
		{"T4: mitad al par por línea (0,01), total acumulado 0,15", "0.01", "0.15"},
	}
	for _, c := range casos {
		f := varias(t, 10, "0.10")
		for i := range f.Detalles {
			f.Detalles[i].Impuestos[0].Valor = d(c.linea)
		}
		fijarTotalIVA(f, d(c.total))
		if hs := validar(f); hs.Err() != nil {
			t.Errorf("%s: el SRI lo autorizó y el SDK lo rechaza: %v", c.nombre, hs.Errores())
		}
	}
	f := varias(t, 1, "1.00")
	f.Detalles[0].Impuestos[0].Valor = d("0.16")
	fijarTotalIVA(f, d("0.16"))
	hs := validar(f)
	exigir(t, hs, "iva.valor", comprobante.Error)
	if len(hs.Errores()) != 1 {
		t.Errorf("T5 debe fallar solo por iva.valor: %v", hs.Errores())
	}
}

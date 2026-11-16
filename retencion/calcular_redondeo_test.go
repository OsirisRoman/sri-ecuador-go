package retencion_test

import (
	"math/rand/v2"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Calcular debe producir importes que su propio Validar acepta aunque la
// base llegue con más de dos decimales: el IVA se calcula sobre la base ya
// redondeada (con 0,0334 al 15 %, sobre la base sin redondear daría 0,01 y
// sobre 0,03 da 0,00).
func TestCalcularRedondeaLaBaseAntesDelImpuesto(t *testing.T) {
	c := base(t, hoy)
	ds := &c.DocsSustento[0]
	ds.TotalSinImpuestos = d("0.0334")
	ds.ImpuestosDocSustento[0].BaseImponible = d("0.0334")
	ds.Retenciones = ds.Retenciones[:1]
	ds.Retenciones[0].BaseImponible = d("0.03")
	ds.Pagos[0].Total = d("0")
	c.Calcular()
	if hs := validar(c); hs.Tiene("iva.valor") {
		t.Errorf("Calcular produjo un IVA que Validar rechaza: base %s, valor %s",
			ds.ImpuestosDocSustento[0].BaseImponible, ds.ImpuestosDocSustento[0].ValorImpuesto)
	}
}

// Propiedad: con bases de hasta cuatro decimales, lo que produce Calcular
// no tiene errores de aritmética en Validar.
func TestCalcularSiempreValida(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	for i := 0; i < 2000; i++ {
		c := base(t, hoy)
		ds := &c.DocsSustento[0]
		b := decimal.Nuevo(int64(1+r.IntN(10_000_000)), r.IntN(5))
		ds.TotalSinImpuestos = b
		ds.ImpuestosDocSustento[0].BaseImponible = b
		ds.Retenciones[0].BaseImponible = b.Porcentaje(decimal.Entero(int64(10 + r.IntN(90))))
		ds.Retenciones[1].BaseImponible = decimal.Nuevo(int64(1+r.IntN(100_000)), r.IntN(4))
		ds.Pagos[0].Total = decimal.Cero
		c.Calcular()
		for _, h := range validar(c).Errores() {
			switch h.Regla {
			case "iva.valor", "retencion.valor", "totales.importe":
				t.Fatalf("caso %d: %v (base %s)", i, h, b)
			}
		}
	}
}

package factura_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// Propiedad: lo que produce Calcular lo acepta Validar. Protege que las
// reglas estrictas de redondeo y de bases (auditoría de v0.1.0, AUD-001 y
// AUD-003) no rechacen importes que el propio SDK calcula.
func TestCalcularSiempreValida(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	codigos := []string{comprobante.IVA0, comprobante.IVA15, comprobante.IVA5, comprobante.IVANoObjeto, comprobante.IVAExento}
	for i := 0; i < 3000; i++ {
		f := base(t)
		f.Detalles = nil
		for j := 0; j < 1+r.IntN(12); j++ {
			cant := decimal.Nuevo(int64(1+r.IntN(5_000_000)), r.IntN(7))
			precio := decimal.Nuevo(int64(1+r.IntN(5_000_000)), r.IntN(7))
			d := factura.LineaIVA("C", "Artículo", cant, precio, codigos[r.IntN(len(codigos))])
			if r.IntN(3) == 0 {
				d.Descuento = cant.Multiplicar(precio).Porcentaje(decimal.Entero(int64(r.IntN(30)))).Redondear(2)
			}
			if r.IntN(5) == 0 {
				d.Impuestos = append([]comprobante.Impuesto{{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3101", Tarifa: decimal.Entero(int64(5 + r.IntN(30)))}}, d.Impuestos...)
			}
			f.Detalles = append(f.Detalles, d)
		}
		f.Calcular()
		hs := validar(f)
		for _, h := range hs.Errores() {
			if h.Regla == "digitos" { // importes que exceden los 14 dígitos del XSD
				continue
			}
			t.Fatalf("caso %d: Calcular produjo un error de validación: %v\n%s", i, h, resumen(f))
		}
	}
}

func resumen(f *factura.Factura) string {
	s := ""
	for _, d := range f.Detalles {
		s += fmt.Sprintf("  %s × %s − %s = %s; impuestos %+v\n", d.Cantidad, d.PrecioUnitario, d.Descuento, d.PrecioTotalSinImpuesto, d.Impuestos)
	}
	return s
}

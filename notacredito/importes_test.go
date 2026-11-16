package notacredito_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
)

// AUD-001: la base del IVA de un detalle de la nota de crédito debe ser el
// total de la línea más su ICE.
func TestBaseIVAIncoherenteConLaLinea(t *testing.T) {
	n := base(t)
	if hs := validar(n); hs.Err() != nil {
		t.Fatal(hs)
	}
	imp := &n.Detalles[0].Impuestos[0]
	imp.BaseImponible = d("1.00")
	imp.Valor = d("0.15")
	n.InfoNotaCredito.TotalConImpuestos[0].BaseImponible = d("1.00")
	n.InfoNotaCredito.TotalConImpuestos[0].Valor = d("0.15")
	n.InfoNotaCredito.ValorModificacion = n.InfoNotaCredito.TotalSinImpuestos.Sumar(d("0.15"))
	hs := validar(n)
	if !hs.Tiene("iva.base") {
		t.Errorf("falta iva.base: %v", hs)
	}
}

// AUD-001: un solo IVA por detalle.
func TestUnSoloIVAPorDetalle(t *testing.T) {
	n := base(t)
	n.Detalles[0].Impuestos = append(n.Detalles[0].Impuestos, comprobante.Impuesto{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA0})
	n.Calcular()
	if hs := validar(n); !hs.Tiene("iva.unico") {
		t.Errorf("falta iva.unico: %v", hs)
	}
}

package notacredito_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
)

// AUD-004: una nota de crédito creada con DesdeFactura que devuelve 50
// veces lo facturado (la auditoría la encontró sin hallazgos).
func TestDevuelveMasDeLoFacturado(t *testing.T) {
	n := base(t)
	n.Detalles[0].Cantidad = d("50")
	n.Calcular()
	hs := validar(n)
	for _, regla := range []string{"origen.valor", "origen.base", "origen.cantidad"} {
		if !hs.Tiene(regla) {
			t.Errorf("falta %s: %v", regla, hs)
		}
	}
}

// Las devoluciones parciales sucesivas se acumulan si se informan las
// anteriores.
func TestNotasDeCreditoAnteriores(t *testing.T) {
	f := ejemplo.Factura(ayer, "000000045")
	primera := notacredito.DesdeFactura(f, "Devolución parcial")
	primera.Detalles[0].Cantidad = d("0.6")
	primera.Calcular()
	segunda := notacredito.DesdeFactura(f, "Devolución parcial")
	segunda.InfoTributaria.Secuencial = "000000008"
	segunda.InfoNotaCredito.FechaEmision = hoy
	segunda.Detalles[0].Cantidad = d("0.6")
	segunda.Calcular()
	segunda.GenerarClaveAcceso("12345678")
	if hs := validar(segunda); hs.Err() != nil {
		t.Fatalf("sola, la segunda es válida: %v", hs)
	}
	segunda.AsociarFactura(f, primera)
	hs := validar(segunda)
	if !hs.Tiene("origen.cantidad") || !hs.Tiene("origen.valor") {
		t.Errorf("1,2 unidades devueltas de 1 facturada no se detectaron: %v", hs)
	}
}

// ValidarContra comprueba la referencia al documento y los impuestos.
func TestReferenciaEImpuestosDeLaFactura(t *testing.T) {
	f := ejemplo.Factura(ayer, "000000045")
	n := base(t)
	n.InfoNotaCredito.NumDocModificado = "001-001-000000046"
	n.Detalles[0].Impuestos[0].CodigoPorcentaje = comprobante.IVA0
	n.Calcular()
	hs := n.ValidarContra(f)
	if !hs.Tiene("origen.documento") || !hs.Tiene("origen.impuesto") {
		t.Errorf("hallazgos: %v", hs)
	}
}

// Una nota de crédito leída de XML no conoce su factura: sin asociarla no
// se aplican estas reglas, y al asociarla sí.
func TestNotaLeidaDeXML(t *testing.T) {
	n := base(t)
	n.Detalles[0].Cantidad = d("50")
	n.Calcular()
	doc, err := n.XML()
	if err != nil {
		t.Fatal(err)
	}
	m, err := notacredito.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if validar(m).Tiene("origen.valor") {
		t.Error("sin factura asociada no debería contrastarse")
	}
	m.AsociarFactura(ejemplo.Factura(ayer, "000000045"))
	if !validar(m).Tiene("origen.valor") {
		t.Error("asociada a la factura debería detectarse")
	}
}

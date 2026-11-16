package notacredito_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
)

// facturaDePreciosFinales: 10 unidades a 12,00 con IVA incluido (120,00) y
// tres líneas de 0,50. Con precios sin impuestos sumaría 121,49.
func facturaDePreciosFinales(t *testing.T) *factura.Factura {
	f := ejemplo.Factura(ayer, "000000045")
	f.Detalles = []factura.Detalle{
		factura.LineaPrecioFinal("ARR", "Arriendo", d("10"), d("12.00"), comprobante.IVA15),
		factura.LineaPrecioFinal("SRV-1", "Servicio", d("1"), d("0.50"), comprobante.IVA15),
		factura.LineaPrecioFinal("SRV-2", "Servicio", d("1"), d("0.50"), comprobante.IVA15),
		factura.LineaPrecioFinal("SRV-3", "Servicio", d("1"), d("0.50"), comprobante.IVA15),
	}
	f.Calcular()
	if !f.InfoFactura.ImporteTotal.Igual(d("121.50")) {
		t.Fatalf("importe de la factura = %s", f.InfoFactura.ImporteTotal)
	}
	return f
}

func preparar(t *testing.T, n *notacredito.NotaCredito) *notacredito.NotaCredito {
	t.Helper()
	n.InfoNotaCredito.FechaEmision = hoy
	n.InfoTributaria.Secuencial = "000000007"
	n.Calcular()
	if _, err := n.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return n
}

// La nota de crédito de una factura de precios finales la revierte por el
// mismo importe, total o parcialmente.
func TestNotaDeCreditoDePreciosFinales(t *testing.T) {
	f := facturaDePreciosFinales(t)
	n := preparar(t, notacredito.DesdeFactura(f, "Anulación"))
	if v := n.InfoNotaCredito.ValorModificacion; !v.Igual(d("121.50")) {
		t.Errorf("valorModificacion = %s; se esperaba 121,50", v)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
	doc, err := n.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.Validar(doc); err != nil {
		t.Errorf("XSD: %v", err)
	}

	// Devolución parcial: 3 de las 10 unidades.
	n = notacredito.DesdeFactura(f, "Devolución parcial")
	n.Detalles = n.Detalles[:1]
	n.Detalles[0].Cantidad = d("3")
	preparar(t, n)
	if v := n.InfoNotaCredito.ValorModificacion; !v.Igual(d("36.00")) {
		t.Errorf("valorModificacion = %s; se esperaba 36,00", v)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
}

// Una factura leída de XML no conserva el modo. Si la aplicación no lo
// declara, la nota de crédito se calcula con precios sin impuestos, difiere
// de la factura y Validar lo advierte; si lo declara, coincide.
func TestNotaDeCreditoDeFacturaLeidaDeXML(t *testing.T) {
	doc, err := facturaDePreciosFinales(t).XML()
	if err != nil {
		t.Fatal(err)
	}
	leer := func() *factura.Factura {
		f, err := factura.Parse(doc)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	n := preparar(t, notacredito.DesdeFactura(leer(), "Anulación"))
	if v := n.InfoNotaCredito.ValorModificacion; v.Igual(d("121.50")) {
		t.Fatalf("se esperaba una diferencia de centavos sin declarar el modo; valorModificacion = %s", v)
	}
	hs := validar(n)
	if !hs.Tiene("origen.precio-final") || hs.Err() != nil {
		t.Errorf("se esperaba solo la advertencia origen.precio-final: %v", hs)
	}

	f := leer()
	f.DeclararPreciosFinales()
	n = preparar(t, notacredito.DesdeFactura(f, "Anulación"))
	if v := n.InfoNotaCredito.ValorModificacion; !v.Igual(d("121.50")) {
		t.Errorf("valorModificacion = %s; se esperaba 121,50", v)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}

	// La nota de crédito leída de su XML también necesita el contexto.
	ncDoc, err := n.XML()
	if err != nil {
		t.Fatal(err)
	}
	leida, err := notacredito.Parse(ncDoc)
	if err != nil {
		t.Fatal(err)
	}
	if hs := leida.ValidarCon(notacredito.OpcionesValidacion{Hoy: hoy}); !hs.Tiene("iva.valor") {
		t.Errorf("sin contexto se esperaba iva.valor: %v", hs)
	}
	leida.DeclararPreciosFinales()
	if hs := leida.ValidarCon(notacredito.OpcionesValidacion{Hoy: hoy}); hs.Err() != nil {
		t.Errorf("con el contexto: %v", hs)
	}
}

// Una nota de crédito hecha a mano con un precio final.
func TestNotaDeCreditoLineaPrecioFinal(t *testing.T) {
	n := base(t)
	n.Detalles = []notacredito.Detalle{notacredito.LineaPrecioFinal("SRV-001", "Rebaja", d("1"), d("0.50"), comprobante.IVA15)}
	n.Calcular()
	if imp := n.Detalles[0].Impuestos[0]; !imp.BaseImponible.Igual(d("0.43")) || !imp.Valor.Igual(d("0.07")) || !n.InfoNotaCredito.ValorModificacion.Igual(d("0.50")) {
		t.Errorf("base %s, IVA %s, valor %s", imp.BaseImponible, imp.Valor, n.InfoNotaCredito.ValorModificacion)
	}
	if hs := validar(n); hs.Err() != nil {
		t.Errorf("hallazgos: %v", hs)
	}
	// Un centavo movido de la base al IVA se rechaza.
	n.Detalles[0].Impuestos[0].BaseImponible, n.Detalles[0].Impuestos[0].Valor = d("0.42"), d("0.08")
	n.Detalles[0].PrecioUnitario, n.Detalles[0].PrecioTotalSinImpuesto = d("0.42"), d("0.42")
	if hs := validar(n); !hs.Tiene("precio-final.base") {
		t.Errorf("falta precio-final.base: %v", hs)
	}
}

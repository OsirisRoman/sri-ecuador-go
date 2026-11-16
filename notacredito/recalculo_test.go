package notacredito_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
)

// facturaConICE es una factura de 10 unidades de 100,00 con ICE ad valorem
// del 75 % e IVA del 15 %: importe total 2012,50.
func facturaConICE(t *testing.T) *factura.Factura {
	f := ejemplo.Factura(ayer, "000000045")
	f.Detalles = []factura.Detalle{{CodigoPrincipal: "LIC-1", Descripcion: "Licor", Cantidad: d("10"), PrecioUnitario: d("100.00"),
		Impuestos: []comprobante.Impuesto{
			{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: d("75")},
			{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15},
		}}}
	f.Calcular()
	if !f.InfoFactura.ImporteTotal.Igual(d("2012.50")) {
		t.Fatalf("importe de la factura = %s", f.InfoFactura.ImporteTotal)
	}
	return f
}

func parcial(t *testing.T, f *factura.Factura, cantidad string) *notacredito.NotaCredito {
	t.Helper()
	n := notacredito.DesdeFactura(f, "Devolución parcial")
	n.InfoNotaCredito.FechaEmision = hoy
	n.InfoTributaria.Secuencial = "000000007"
	if !n.InfoNotaCredito.ValorModificacion.Igual(f.InfoFactura.ImporteTotal) {
		t.Fatalf("la nota de crédito total es de %s y la factura de %s", n.InfoNotaCredito.ValorModificacion, f.InfoFactura.ImporteTotal)
	}
	n.Detalles[0].Cantidad = d(cantidad)
	n.Calcular()
	if _, err := n.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return n
}

func exigirParcial(t *testing.T, n *notacredito.NotaCredito) {
	t.Helper()
	imp := n.Detalles[0].Impuestos
	if !imp[0].BaseImponible.Igual(d("100.00")) || !imp[0].Valor.Igual(d("75.00")) || !imp[1].BaseImponible.Igual(d("175.00")) || !imp[1].Valor.Igual(d("26.25")) {
		t.Errorf("impuestos de la línea: %+v", imp)
	}
	if v := n.InfoNotaCredito.ValorModificacion; !v.Igual(d("201.25")) {
		t.Errorf("valorModificacion = %s; se esperaba 201.25", v)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
}

// Segunda auditoría, N-01: una nota de crédito parcial hecha con
// DesdeFactura sobre una factura con ICE devolvía el ICE de toda la
// factura (977,50 por 1 de 10 unidades, en lugar de 201,25).
func TestNotaDeCreditoParcialConICE(t *testing.T) {
	exigirParcial(t, parcial(t, facturaConICE(t), "1"))
}

// Lo habitual es que la aplicación lea la factura autorizada de su XML: no
// conserva memoria de lo que dedujo Calcular.
func TestNotaDeCreditoParcialDeFacturaLeidaDeXML(t *testing.T) {
	doc, err := facturaConICE(t).XML()
	if err != nil {
		t.Fatal(err)
	}
	f, err := factura.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	exigirParcial(t, parcial(t, f, "1"))
}

// Si la factura lleva un ICE indicado por la aplicación (específico, o con
// base propia), DesdeFactura lo copia y lo respeta al recalcular: la
// aplicación debe ajustarlo en una devolución parcial.
func TestNotaDeCreditoRespetaElICEIndicado(t *testing.T) {
	f := facturaConICE(t)
	f.Detalles[0].Impuestos[0] = comprobante.Impuesto{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3011", BaseImponible: d("10"), Valor: d("72.20")}
	f.Calcular()
	n := notacredito.DesdeFactura(f, "Devolución parcial")
	n.Detalles[0].Cantidad = d("1")
	n.Calcular()
	if ice := n.Detalles[0].Impuestos[0]; !ice.Valor.Igual(d("72.20")) {
		t.Errorf("el ICE específico cambió: %+v", ice)
	}
	n.Detalles[0].Impuestos[0].BaseImponible, n.Detalles[0].Impuestos[0].Valor = d("1"), d("7.22")
	n.Calcular()
	if v := n.InfoNotaCredito.ValorModificacion; !v.Igual(d("123.30")) { // 100 + 7,22 + 15 % de 107,22
		t.Errorf("valorModificacion = %s; se esperaba 123.30", v)
	}
}

// Recalcular una nota de crédito construida a mano.
func TestRecalcularNotaDeCredito(t *testing.T) {
	n := base(t)
	n.Detalles = []notacredito.Detalle{{CodigoInterno: "LIC-1", Descripcion: "Licor", Cantidad: d("10"), PrecioUnitario: d("100.00"),
		Impuestos: []comprobante.Impuesto{
			{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: d("75")},
			{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15},
		}}}
	n.Calcular()
	n.Detalles[0].PrecioUnitario = d("10.00")
	n.Detalles[0].Cantidad = d("10")
	n.Calcular()
	if v := n.InfoNotaCredito.ValorModificacion; !v.Igual(d("201.25")) {
		t.Errorf("valorModificacion = %s; se esperaba 201.25", v)
	}
}

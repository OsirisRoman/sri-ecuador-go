package liquidacion_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/liquidacion"
)

// La liquidación de compra comparte los detalles de la factura, también el
// modo de precio final (IVA incluido).
func TestPrecioFinal(t *testing.T) {
	l := base(t, hoy)
	l.Detalles = []liquidacion.Detalle{
		liquidacion.LineaPrecioFinal("FLETE", "Servicio de acarreo", d("1"), d("12.00"), comprobante.IVA15),
		liquidacion.LineaPrecioFinal("CARGA", "Estiba", d("10"), d("0.50"), comprobante.IVA15),
	}
	l.Calcular()
	info := l.InfoLiquidacionCompra
	if !info.ImporteTotal.Igual(d("17.00")) || !info.Pagos[0].Total.Igual(d("17.00")) || !info.TotalSinImpuestos.Igual(d("14.78")) {
		t.Errorf("importe %s, pago %s, subtotal %s", info.ImporteTotal, info.Pagos[0].Total, info.TotalSinImpuestos)
	}
	if hs := validar(l); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
	doc, err := l.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.Validar(doc); err != nil {
		t.Errorf("XSD: %v", err)
	}
	leida, err := liquidacion.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if hs := leida.ValidarCon(liquidacion.OpcionesValidacion{Hoy: hoy}); !hs.Tiene("iva.valor") {
		t.Errorf("sin contexto se esperaba iva.valor: %v", hs)
	}
	leida.DeclararPreciosFinales()
	if hs := leida.ValidarCon(liquidacion.OpcionesValidacion{Hoy: hoy}); hs.Err() != nil {
		t.Errorf("con el contexto: %v", hs)
	}
}

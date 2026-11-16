package liquidacion_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/liquidacion"
)

// Segunda auditoría, N-01: corregir un detalle y recalcular actualiza el
// importe y el pago único.
func TestRecalcularTrasCorregir(t *testing.T) {
	l := base(t, hoy)
	antes := l.InfoLiquidacionCompra.ImporteTotal
	if !l.InfoLiquidacionCompra.Pagos[0].Total.Igual(antes) {
		t.Fatalf("pago %s, importe %s", l.InfoLiquidacionCompra.Pagos[0].Total, antes)
	}
	l.Detalles[1].Cantidad = d("3") // 3 × 20,00 al 15 %
	l.Calcular()
	info := l.InfoLiquidacionCompra
	if quiere := antes.Sumar(d("46.00")); !info.ImporteTotal.Igual(quiere) || !info.Pagos[0].Total.Igual(quiere) {
		t.Errorf("importe %s y pago %s; se esperaba %s", info.ImporteTotal, info.Pagos[0].Total, quiere)
	}
	if hs := validar(l); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}

	// También si pasó por XML, y sin tocar un pago que no es el importe.
	doc, err := l.XML()
	if err != nil {
		t.Fatal(err)
	}
	leida, err := liquidacion.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	leida.Detalles[1].Cantidad = d("1")
	leida.Calcular()
	if info := leida.InfoLiquidacionCompra; !info.ImporteTotal.Igual(antes) || !info.Pagos[0].Total.Igual(antes) {
		t.Errorf("importe %s y pago %s; se esperaba %s", info.ImporteTotal, info.Pagos[0].Total, antes)
	}
	leida.InfoLiquidacionCompra.Pagos[0].Total = d("5.00")
	leida.Detalles[1].Cantidad = d("2")
	leida.Calcular()
	if p := leida.InfoLiquidacionCompra.Pagos[0].Total; !p.Igual(d("5.00")) {
		t.Errorf("el pago indicado cambió a %s", p)
	}
	if hs := validar(leida); !hs.Tiene("pagos.suma") {
		t.Errorf("falta pagos.suma: %v", hs)
	}
}

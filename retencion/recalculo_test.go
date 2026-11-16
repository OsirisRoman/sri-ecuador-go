package retencion_test

import "testing"

// Segunda auditoría, N-01: corregir el documento de sustento y recalcular
// actualiza el importe, los valores retenidos y el pago único.
func TestRecalcularTrasCorregirElSustento(t *testing.T) {
	c := base(t, hoy)
	ds := &c.DocsSustento[0]
	if !ds.ImporteTotal.Igual(d("230.00")) || !ds.Pagos[0].Total.Igual(d("230.00")) {
		t.Fatalf("importe %s, pago %s", ds.ImporteTotal, ds.Pagos[0].Total)
	}
	ds.TotalSinImpuestos, ds.ImpuestosDocSustento[0].BaseImponible = d("100.00"), d("100.00")
	ds.Retenciones[0].BaseImponible, ds.Retenciones[1].BaseImponible = d("100.00"), d("15.00")
	c.Calcular()
	if !ds.ImporteTotal.Igual(d("115.00")) || !ds.Pagos[0].Total.Igual(d("115.00")) ||
		!ds.Retenciones[0].ValorRetenido.Igual(d("2.00")) || !ds.Retenciones[1].ValorRetenido.Igual(d("4.50")) {
		t.Errorf("importe %s, pago %s, retenido %s y %s", ds.ImporteTotal, ds.Pagos[0].Total, ds.Retenciones[0].ValorRetenido, ds.Retenciones[1].ValorRetenido)
	}
	if hs := validar(c); hs.Err() != nil {
		t.Errorf("hallazgos: %v", hs)
	}
	// Un pago que no es el importe lo indicó la aplicación.
	ds.Pagos[0].Total = d("50.00")
	ds.TotalSinImpuestos, ds.ImpuestosDocSustento[0].BaseImponible = d("200.00"), d("200.00")
	c.Calcular()
	if !ds.Pagos[0].Total.Igual(d("50.00")) {
		t.Errorf("el pago indicado cambió a %s", ds.Pagos[0].Total)
	}
}

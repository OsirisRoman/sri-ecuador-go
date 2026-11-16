package retencion_test

import "testing"

// AUD-003: el valor retenido solo admite el redondeo al centavo más
// cercano, no un centavo de diferencia.
func TestValorRetenidoSinTolerancia(t *testing.T) {
	c := base(t, hoy)
	r := &c.DocsSustento[0].Retenciones[0] // 2 % de 200,00 = 4,00
	r.ValorRetenido = d("4.01")
	if hs := validar(c); !hs.Tiene("retencion.valor") {
		t.Errorf("falta retencion.valor: %v", hs)
	}
	c = base(t, hoy)
	c.DocsSustento[0].ImpuestosDocSustento[0].ValorImpuesto = d("30.01")
	if hs := validar(c); !hs.Tiene("iva.valor") {
		t.Errorf("falta iva.valor: %v", hs)
	}
}

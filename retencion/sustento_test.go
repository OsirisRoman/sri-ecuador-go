package retencion_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
)

func severidad(hs comprobante.Hallazgos, regla string) (comprobante.Severidad, bool) {
	for _, h := range hs {
		if h.Regla == regla {
			return h.Severidad, true
		}
	}
	return 0, false
}

// AUD-004: bases de retención mayores que el documento de sustento
// (auditoría: 19 500 retenidos sobre un sustento de 230).
func TestBasesDeRetencionMayoresQueElSustento(t *testing.T) {
	c := base(t, hoy)
	c.DocsSustento[0].Retenciones[0].BaseImponible = d("900000.00") // renta; el sustento es 200 sin IVA
	c.DocsSustento[0].Retenciones[1].BaseImponible = d("5000.00")   // IVA; el sustento tiene 30 de IVA
	c.Calcular()
	hs := validar(c)
	for _, regla := range []string{"retencion.base-renta", "retencion.base-iva"} {
		if s, ok := severidad(hs, regla); !ok || s != comprobante.Advertencia {
			t.Errorf("%s: se esperaba advertencia; hallazgos: %v", regla, hs)
		}
	}
	estricto := c.ValidarCon(retencion.OpcionesValidacion{Hoy: hoy, SustentoEstricto: true})
	for _, regla := range []string{"retencion.base-renta", "retencion.base-iva"} {
		if s, ok := severidad(estricto, regla); !ok || s != comprobante.Error {
			t.Errorf("%s en modo estricto: se esperaba error; hallazgos: %v", regla, estricto)
		}
	}
}

// AUD-004: la misma retención de IVA dos veces supera el IVA del sustento.
func TestRetencionDuplicada(t *testing.T) {
	c := base(t, hoy)
	ds := &c.DocsSustento[0]
	ds.Retenciones = append(ds.Retenciones, ds.Retenciones[1])
	c.Calcular()
	if _, ok := severidad(validar(c), "retencion.base-iva"); !ok {
		t.Errorf("la retención duplicada no se detectó")
	}
}

// Las bases que cuadran con el sustento no producen hallazgos.
func TestBasesDeRetencionCoherentes(t *testing.T) {
	if hs := validar(base(t, hoy)); len(hs) != 0 {
		t.Errorf("hallazgos inesperados: %v", hs)
	}
}

// AUD-004: bases de IVA del sustento mayores que su total.
func TestBasesDeIVADelSustento(t *testing.T) {
	c := base(t, hoy)
	c.DocsSustento[0].ImpuestosDocSustento[0].BaseImponible = d("300.00")
	c.Calcular()
	if _, ok := severidad(validar(c), "sustento.base-iva"); !ok {
		t.Errorf("falta sustento.base-iva")
	}
}

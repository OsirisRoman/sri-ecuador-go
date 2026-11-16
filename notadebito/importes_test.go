package notadebito_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
)

// AUD-001: en la nota de débito las bases de IVA se reparten entre los
// motivos; si suman menos que el total, parte del valor no tiene tarifa.
func TestBasesDeIVAParciales(t *testing.T) {
	n := base(t, hoy)
	if hs := validar(n); len(hs) != 0 {
		t.Fatal(hs)
	}
	n.InfoNotaDebito.Impuestos[0].BaseImponible = d("5.00")
	n.InfoNotaDebito.Impuestos[0].Valor = d("0.75")
	n.InfoNotaDebito.ValorTotal = n.InfoNotaDebito.TotalSinImpuestos.Sumar(d("0.75"))
	n.InfoNotaDebito.Pagos[0].Total = n.InfoNotaDebito.ValorTotal
	hs := validar(n)
	if !hs.Tiene("iva.base") || hs.Err() != nil {
		t.Errorf("se esperaba solo la advertencia iva.base: %v", hs)
	}
}

// Dos motivos con tarifas distintas: las bases suman el total y es válido.
func TestBasesDeIVARepartidas(t *testing.T) {
	n := base(t, hoy)
	n.InfoNotaDebito.Impuestos = []notadebito.Impuesto{notadebito.ImpuestoIVA(comprobante.IVA15), notadebito.ImpuestoIVA(comprobante.IVA0)}
	n.InfoNotaDebito.Impuestos[0].BaseImponible = d("12.35")
	n.InfoNotaDebito.Impuestos[1].BaseImponible = d("5.00")
	n.Calcular()
	if hs := validar(n); hs.Err() != nil || hs.Tiene("iva.base") {
		t.Errorf("hallazgos inesperados: %v", hs)
	}
}

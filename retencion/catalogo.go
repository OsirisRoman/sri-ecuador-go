package retencion

import (
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// ConceptoRenta es una fila de la tabla 3.10 del Catálogo ATS del SRI:
// «Conceptos de retención en la fuente de impuesto a la renta».
//
// # Este catálogo caduca
//
// Los porcentajes cambian con la normativa (el SRI publicó tres versiones
// de la tabla solo entre marzo y agosto de 2026). La copia incluida es la
// vigente en CatalogoRentaDescarga y rige desde CatalogoRentaVigencia. El
// SRI contrasta el porcentaje con su parametrización al autorizar (error 52
// si no coincide), de modo que la referencia definitiva es siempre el SRI.
// Por eso Validar reporta las discrepancias con este catálogo como
// advertencias, no como errores, salvo que se pida lo contrario con
// OpcionesValidacion.CatalogoRentaEstricto.
//
// El procedimiento para actualizarlo está en docs/actualizar-catalogos.md.
type ConceptoRenta struct {
	// Codigo es el valor de <codigoRetencion> (sin espacios).
	Codigo   string
	Concepto string
	// Texto es el porcentaje tal como lo escribe el catálogo, que a veces
	// incluye condiciones ("0, 25, 37 (0 por casuística de CDI)").
	Texto string
	// Porcentajes son los valores admitidos cuando el catálogo los
	// enumera. Vacío si no se pueden comprobar.
	Porcentajes []string
	// Minimo y Maximo delimitan el porcentaje cuando el catálogo da un
	// rango ("0 al 37").
	Minimo, Maximo string
}

var indiceRenta = func() map[string]int {
	m := make(map[string]int, len(conceptosRenta))
	for i, c := range conceptosRenta {
		m[c.Codigo] = i
	}
	return m
}()

// ConceptosRenta devuelve una copia del catálogo incluido, en el orden del
// SRI.
func ConceptosRenta() []ConceptoRenta {
	out := make([]ConceptoRenta, len(conceptosRenta))
	copy(out, conceptosRenta)
	return out
}

// BuscarConceptoRenta busca un código de retención de renta en el catálogo
// incluido. Los espacios del código se ignoran.
func BuscarConceptoRenta(codigo string) (ConceptoRenta, bool) {
	i, ok := indiceRenta[strings.ReplaceAll(codigo, " ", "")]
	if !ok {
		return ConceptoRenta{}, false
	}
	return conceptosRenta[i], true
}

// Comprobable informa si el catálogo permite comprobar el porcentaje de
// este concepto (enumera valores o da un rango).
func (c ConceptoRenta) Comprobable() bool {
	return len(c.Porcentajes) > 0 || c.Minimo != ""
}

// Admite informa si el catálogo admite el porcentaje p para este concepto.
// Si el concepto no es Comprobable devuelve true.
func (c ConceptoRenta) Admite(p decimal.Decimal) bool {
	if !c.Comprobable() {
		return true
	}
	for _, s := range c.Porcentajes {
		if v, err := decimal.Parse(s); err == nil && v.Igual(p) {
			return true
		}
	}
	if c.Minimo != "" {
		mn, err1 := decimal.Parse(c.Minimo)
		mx, err2 := decimal.Parse(c.Maximo)
		return err1 == nil && err2 == nil && p.Cmp(mn) >= 0 && p.Cmp(mx) <= 0
	}
	return false
}

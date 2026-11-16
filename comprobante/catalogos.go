package comprobante

import "maps"

// Instantáneas privadas de los catálogos, tomadas al iniciar el paquete. Las
// validaciones y los cálculos del SDK usan estas copias, no las variables
// exportadas: así, una aplicación que modifique TarifasIVA, FormasPago o
// Paises (por error o en una prueba) no altera el resultado de otras
// goroutines ni provoca una carrera de datos con ellas.
var (
	tarifasIVA = maps.Clone(TarifasIVA)
	formasPago = maps.Clone(FormasPago)
	paises     = maps.Clone(Paises)
)

// TarifaIVADe devuelve la fila de la Tabla 17 de un código de porcentaje de
// IVA, tal como la usa el SDK.
func TarifaIVADe(codigoPorcentaje string) (TarifaIVA, bool) {
	t, ok := tarifasIVA[codigoPorcentaje]
	return t, ok
}

// FormaPagoDe devuelve la descripción de una forma de pago vigente de la
// Tabla 24.
func FormaPagoDe(codigo string) (string, bool) {
	d, ok := formasPago[codigo]
	return d, ok
}

// PaisDe devuelve el nombre de un país de la Tabla 25.
func PaisDe(codigo string) (string, bool) {
	p, ok := paises[codigo]
	return p, ok
}

package comprobante

import (
	"time"
)

// periodo es un intervalo de fechas de emisión, ambos extremos incluidos;
// un extremo cero significa «sin límite».
type periodo struct{ desde, hasta Fecha }

func (p periodo) incluye(f Fecha) bool {
	if !p.desde.EsCero() && f.Time().Before(p.desde.Time()) {
		return false
	}
	return p.hasta.EsCero() || !f.Time().After(p.hasta.Time())
}

func fecha(a int, m time.Month, d int) Fecha { return NuevaFecha(a, m, d) }

// vigenciaIVA son los períodos en que cada tarifa de la Tabla 17 fue la
// tarifa general del IVA. La ficha técnica no publica estas fechas, así que
// salen de la ley:
//
//   - 14 %: del 1 de junio de 2016 al 31 de mayo de 2017 (Ley Orgánica de
//     Solidaridad y de Corresponsabilidad Ciudadana, R.O. S. 759 de
//     20-05-2016, disposición transitoria primera).
//   - 12 %: antes y después de ese período, hasta el 31 de marzo de 2024.
//   - 15 %: desde el 1 de abril de 2024 (Ley Orgánica para Enfrentar el
//     Conflicto Armado Interno, R.O. 3S-516 de 12-03-2024, y Decreto
//     Ejecutivo 198).
//
// Solo figuran las tarifas generales. Las demás (0 %, 5 %, 13 %, no objeto,
// exento, diferenciado) dependen del bien, de un decreto o de la actividad, y
// el SDK no tiene datos para juzgarlas. La lista caduca: ver
// docs/actualizar-catalogos.md.
var vigenciaIVA = map[string][]periodo{
	IVA12: {{hasta: fecha(2016, 5, 31)}, {desde: fecha(2017, 6, 1), hasta: fecha(2024, 3, 31)}},
	IVA14: {{desde: fecha(2016, 6, 1), hasta: fecha(2017, 5, 31)}},
	IVA15: {{desde: fecha(2024, 4, 1)}},
}

// IVAVigente informa si el código de la Tabla 17 era la tarifa general del
// IVA en la fecha dada. Devuelve conocido = false para los códigos de los que
// el SDK no tiene fechas.
func IVAVigente(codigoPorcentaje string, f Fecha) (vigente, conocido bool) {
	ps, ok := vigenciaIVA[codigoPorcentaje]
	if !ok || f.EsCero() {
		return false, false
	}
	for _, p := range ps {
		if p.incluye(f) {
			return true, true
		}
	}
	return false, true
}

// VigenciaIVA advierte cuando un IVA de tarifa general no estaba vigente en
// la fecha de emisión (regla iva.vigencia). Es una advertencia y no un
// error: un comprobante puede aplicar legítimamente otra tarifa (por
// ejemplo, una nota de crédito que corrige una venta anterior, o las zonas
// con régimen especial), y los cambios futuros de tarifa no los conoce esta
// versión del SDK.
func (v *Validador) VigenciaIVA(campo string, imp Impuesto, emision Fecha) {
	if imp.Codigo != ImpuestoIVA {
		return
	}
	if vigente, conocido := IVAVigente(imp.CodigoPorcentaje, emision); conocido && !vigente {
		v.Advertirf(campo, "iva.vigencia", "la tarifa %s (código %s) no era la tarifa general del IVA el %s; revise la fecha de emisión o la tarifa (ver docs/actualizar-catalogos.md)",
			descripcionIVA(imp.CodigoPorcentaje), imp.CodigoPorcentaje, emision)
	}
}

func descripcionIVA(codigo string) string {
	if t, ok := tarifasIVA[codigo]; ok {
		return t.Descripcion
	}
	return codigo
}

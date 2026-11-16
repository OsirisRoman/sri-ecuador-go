package comprobante

import (
	"fmt"
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// TotalLinea devuelve cantidad × precioUnitario − descuento con 2 decimales:
// el <precioTotalSinImpuesto> de un detalle.
func TotalLinea(cantidad, precioUnitario, descuento decimal.Decimal) decimal.Decimal {
	return cantidad.Multiplicar(precioUnitario).Restar(descuento).Redondear(2)
}

// CalcularImpuestos completa los impuestos de un detalle cuyo total sin
// impuestos es total, modificando el slice recibido. Equivale a
// RecalcularImpuestos sin total anterior.
func CalcularImpuestos(total decimal.Decimal, impuestos []Impuesto) {
	RecalcularImpuestos(decimal.Cero, total, impuestos)
}

// RecalcularImpuestos completa los impuestos de un detalle cuyo total sin
// impuestos es total, modificando el slice recibido. anterior es el total
// que tenía el detalle antes de este cálculo (cero si aún no lo tenía).
//
//   - IVA: se deduce siempre. La tarifa se toma de la Tabla 17 cuando el
//     código la determina; la base es el total de la línea más el ICE de
//     la misma línea, que integra la base imponible del IVA; el valor es
//     base × tarifa.
//   - ICE e IRBPNR: se deduce lo que la aplicación no indicó. Una base en
//     cero pasa a ser el total de la línea; un valor en cero, con tarifa,
//     pasa a ser base × tarifa %. Una base o un valor indicados se
//     respetan: la base del ICE puede ser el precio de venta al público, y
//     el valor puede tener un componente específico por unidad.
//
// Al recalcular, lo que dedujo el cálculo anterior se vuelve a deducir con
// los datos actuales, y lo que indicó la aplicación se sigue respetando.
// Cada Impuesto recuerda qué se dedujo de él; esa memoria se copia con el
// valor, pero no se serializa. Un impuesto que no la tiene (leído de XML, o
// construido con sus importes ya calculados) se considera deducido en lo que
// coincide con la deducción: una base igual a anterior y un valor igual a
// base × tarifa %. Para que Calcular deduzca un importe que antes se indicó,
// basta ponerlo a cero.
func RecalcularImpuestos(anterior, total decimal.Decimal, impuestos []Impuesto) {
	ice := decimal.Cero
	for j := range impuestos {
		imp := &impuestos[j]
		if imp.Codigo == ImpuestoIVA {
			continue
		}
		imp.derivado.Aplicar(anterior, total, imp.Tarifa, &imp.BaseImponible, &imp.Valor)
		if imp.Codigo == ImpuestoICE {
			ice = ice.Sumar(imp.Valor)
		}
	}
	for j := range impuestos {
		imp := &impuestos[j]
		if imp.Codigo != ImpuestoIVA {
			continue
		}
		if t, ok := tarifasIVA[imp.CodigoPorcentaje]; ok && t.Fija {
			imp.Tarifa = t.Tarifa
		}
		imp.BaseImponible = total.Sumar(ice).Redondear(2)
		imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa).Redondear(2)
		imp.derivado.Deducido(imp.BaseImponible, imp.Valor, true)
	}
}

// AsignarPagoUnico mantiene el total de un pago único igual al importe del
// comprobante: si hay un solo pago y su total es cero o es anterior (el
// importe que tenía el comprobante antes de este cálculo), se le asigna
// importe. Un pago con otro total, o varios pagos, los indicó la
// aplicación y no se tocan.
func AsignarPagoUnico(pagos []Pago, anterior, importe decimal.Decimal) {
	if len(pagos) == 1 && (pagos[0].Total.EsCero() || pagos[0].Total.Igual(anterior)) {
		pagos[0].Total = importe
	}
}

// GrupoImpuesto acumula los impuestos de los detalles que comparten código,
// código de porcentaje y tarifa: una fila de <totalConImpuestos>.
type GrupoImpuesto struct {
	Codigo, CodigoPorcentaje string
	Tarifa                   decimal.Decimal
	// Base es la suma de las bases imponibles de las líneas.
	Base decimal.Decimal
	// SumaLineas es la suma de los valores de impuesto de cada línea, ya
	// redondeados.
	SumaLineas decimal.Decimal
}

// Valor es el impuesto total del grupo. Para el IVA se calcula sobre la
// base acumulada (base × tarifa), que es como se liquida el impuesto del
// comprobante; para ICE e IRBPNR, que pueden tener tarifas específicas por
// unidad, se suman los valores de las líneas.
func (g GrupoImpuesto) Valor() decimal.Decimal {
	if g.Codigo == ImpuestoIVA {
		return g.Base.Porcentaje(g.Tarifa).Redondear(2)
	}
	return g.SumaLineas.Redondear(2)
}

// AgruparImpuestos agrupa los impuestos de todos los detalles en orden de
// aparición. La tarifa forma parte de la clave porque el IVA diferenciado
// (código 8) puede aparecer con tarifas distintas bajo el mismo código.
func AgruparImpuestos(porLinea ...[]Impuesto) []GrupoImpuesto {
	type clave struct{ codigo, porcentaje, tarifa string }
	var orden []GrupoImpuesto
	indice := map[clave]int{}
	for _, impuestos := range porLinea {
		for _, imp := range impuestos {
			k := clave{imp.Codigo, imp.CodigoPorcentaje, imp.Tarifa.Redondear(2).String()}
			i, ok := indice[k]
			if !ok {
				i = len(orden)
				indice[k] = i
				orden = append(orden, GrupoImpuesto{Codigo: imp.Codigo, CodigoPorcentaje: imp.CodigoPorcentaje, Tarifa: imp.Tarifa})
			}
			orden[i].Base = orden[i].Base.Sumar(imp.BaseImponible)
			orden[i].SumaLineas = orden[i].SumaLineas.Sumar(imp.Valor)
		}
	}
	return orden
}

// medioCentavo es la mayor distancia entre un importe exacto y su valor
// redondeado al centavo más cercano.
var medioCentavo = decimal.MustParse("0.005")

// ImporteRedondeado informa si declarado es el importe exacto redondeado al
// centavo más cercano: |declarado − exacto| ≤ 0,005. Admite los dos
// criterios de desempate habituales (mitad hacia arriba y mitad al par),
// que solo difieren cuando el exacto termina justo en medio centavo, pero
// rechaza cualquier diferencia de un centavo o más.
//
// La ficha técnica no publica una tolerancia: el código 52 («error en
// diferencias») se produce «cuando existe error en los cálculos del
// comprobante» (sección 11). Por eso el SDK no admite más diferencia que la
// que produce el propio redondeo.
func ImporteRedondeado(declarado, exacto decimal.Decimal) bool {
	return declarado.Restar(exacto).Abs().Cmp(medioCentavo) <= 0
}

// Numero valida las facetas de un xsd:decimal del SRI: no negativo, con un
// máximo de dígitos totales y de decimales. Los dígitos se cuentan como
// están escritos, que es más estricto que el XSD (cuyo totalDigits ignora
// los ceros finales): una tarifa de ICE del 100 % se escribe «100», no
// «100.00».
func (v *Validador) Numero(campo string, d decimal.Decimal, totalDigitos, decimales int) {
	if d.Signo() < 0 {
		v.Errorf(campo, "negativo", "no puede ser negativo (%s)", d)
	}
	enteros, frac := d.Digitos()
	if frac > decimales {
		v.Errorf(campo, "decimales", "%s tiene %d decimales; el máximo es %d", d, frac, decimales)
	}
	if enteros+frac > totalDigitos {
		v.Errorf(campo, "digitos", "%s tiene %d dígitos; el máximo es %d", d, enteros+frac, totalDigitos)
	}
}

// ImpuestoLinea valida un <impuesto> de un detalle; p es el prefijo del
// campo, terminado en punto.
func (v *Validador) ImpuestoLinea(p string, imp Impuesto) { v.impuestoLinea(p, imp, true) }

// impuestoLinea es ImpuestoLinea; conValor indica si se contrasta el valor
// del IVA con base × tarifa (iva.valor). No se hace en un detalle definido
// por su precio final, que se contrasta con ese precio.
func (v *Validador) impuestoLinea(p string, imp Impuesto, conValor bool) {
	v.Numero(p+"tarifa", imp.Tarifa, 4, 2)
	v.Numero(p+"baseImponible", imp.BaseImponible, 14, 2)
	v.Numero(p+"valor", imp.Valor, 14, 2)
	if !SoloDigitos(imp.CodigoPorcentaje) || len(imp.CodigoPorcentaje) > 4 {
		v.Errorf(p+"codigoPorcentaje", "formato", "debe tener de 1 a 4 dígitos")
	}
	switch imp.Codigo {
	case ImpuestoIVA:
		t, ok := tarifasIVA[imp.CodigoPorcentaje]
		if !ok {
			v.Errorf(p+"codigoPorcentaje", "catalogo", "código de IVA %q desconocido (Tabla 17)", imp.CodigoPorcentaje)
			return
		}
		if t.Fija && !imp.Tarifa.Igual(t.Tarifa) {
			v.Errorf(p+"tarifa", "iva.tarifa", "el código %s corresponde a %s; la tarifa declarada es %s", t.Codigo, t.Descripcion, imp.Tarifa)
		}
		if exacto := imp.BaseImponible.Porcentaje(imp.Tarifa); conValor && !ImporteRedondeado(imp.Valor, exacto) {
			v.Errorf(p+"valor", "iva.valor", "es %s pero base × tarifa = %s", imp.Valor, exacto.Redondear(2))
		}
	case ImpuestoICE, ImpuestoIRBPNR:
		// Tarifas específicas o mixtas: no se recalculan aquí.
	default:
		v.Errorf(p+"codigo", "catalogo", "código de impuesto %q desconocido (Tabla 16: 2 IVA, 3 ICE, 5 IRBPNR)", imp.Codigo)
	}
}

// ImpuestosDetalle valida los impuestos de un detalle de venta (factura,
// liquidación de compra o nota de crédito) en conjunto con el total de la
// línea, además de cada impuesto por separado (ImpuestoLinea). p es la ruta
// del detalle terminada en punto y totalLinea su precioTotalSinImpuesto.
//
//   - iva.base: la base del IVA es el total de la línea más el ICE de la
//     misma línea, porque «el valor del ICE formará parte de la base
//     imponible del IVA» (ficha técnica, Tabla 18; art. 58 de la LRTI) y el
//     facturador del SRI calcula así el IVA (sección 9.16, campo 8). El
//     IRBPNR no forma parte de esa base.
//   - iva.unico: un detalle no puede llevar más de un IVA; cada bien o
//     servicio tiene una sola tarifa de la Tabla 17.
//   - iva.ausente (advertencia): un detalle sin IVA. Lo que no grava IVA se
//     declara con tarifa 0 %, no objeto (6) o exento (7); el XSD no lo
//     exige y no se ha comprobado si el SRI lo rechaza.
//
// No se recalculan la base ni el valor del ICE ni del IRBPNR: dependen de
// tarifas específicas, del precio de venta al público o de la cantidad.
func (v *Validador) ImpuestosDetalle(p string, totalLinea decimal.Decimal, impuestos []Impuesto) {
	v.impuestosDetalle(p, totalLinea, impuestos, true)
}

func (v *Validador) impuestosDetalle(p string, totalLinea decimal.Decimal, impuestos []Impuesto, conValor bool) {
	ice := decimal.Cero
	for _, imp := range impuestos {
		if imp.Codigo == ImpuestoICE {
			ice = ice.Sumar(imp.Valor)
		}
	}
	nIVA := 0
	for j, imp := range impuestos {
		q := fmt.Sprintf("%simpuestos[%d].", p, j)
		v.impuestoLinea(q, imp, conValor)
		if imp.Codigo != ImpuestoIVA {
			continue
		}
		if nIVA++; nIVA == 2 {
			v.Errorf(q+"codigo", "iva.unico", "el detalle declara más de un IVA; cada bien o servicio tiene una sola tarifa (Tabla 17)")
		}
		if base := totalLinea.Sumar(ice); !imp.BaseImponible.Igual(base) {
			v.Errorf(q+"baseImponible", "iva.base", "es %s pero debe ser el total de la línea más su ICE = %s", imp.BaseImponible, base.Redondear(2))
		}
	}
	if len(impuestos) > 0 && nIVA == 0 {
		v.Advertirf(p+"impuestos", "iva.ausente", "el detalle no declara IVA; lo que no grava IVA se declara con tarifa 0 %%, no objeto (6) o exento (7)")
	}
}

// TotalesImpuestos contrasta un <totalConImpuestos> con los impuestos de
// los detalles y devuelve la suma de los valores declarados. campo es la
// ruta del bloque, por ejemplo "infoFactura.totalConImpuestos".
func (v *Validador) TotalesImpuestos(campo string, declarados []TotalImpuesto, porLinea ...[]Impuesto) decimal.Decimal {
	if len(declarados) == 0 {
		v.Errorf(campo, "requerido", "debe contener al menos un totalImpuesto")
	}
	type par struct{ codigo, porcentaje string }
	// Varios grupos (IVA diferenciado con tarifas distintas) pueden
	// corresponder a un mismo par código/porcentaje.
	bases := map[par]decimal.Decimal{}
	recalculado := map[par]decimal.Decimal{}
	exacto := map[par]decimal.Decimal{}
	sumaLineas := map[par]decimal.Decimal{}
	tarifas := map[par][]decimal.Decimal{}
	var orden []par
	for _, g := range AgruparImpuestos(porLinea...) {
		k := par{g.Codigo, g.CodigoPorcentaje}
		if _, visto := bases[k]; !visto {
			orden = append(orden, k)
		}
		bases[k] = bases[k].Sumar(g.Base)
		recalculado[k] = recalculado[k].Sumar(g.Valor())
		exacto[k] = exacto[k].Sumar(g.Base.Porcentaje(g.Tarifa))
		sumaLineas[k] = sumaLineas[k].Sumar(g.SumaLineas)
		tarifas[k] = append(tarifas[k], g.Tarifa)
	}
	acumulados := map[par]TotalImpuesto{}
	total := decimal.Cero
	for i, t := range declarados {
		p := fmt.Sprintf("%s[%d].", campo, i)
		k := par{t.Codigo, t.CodigoPorcentaje}
		v.Numero(p+"baseImponible", t.BaseImponible, 14, 2)
		v.Numero(p+"valor", t.Valor, 14, 2)
		v.DevolucionIVA(p+"valorDevolucionIva", t.ValorDevolucionIva)
		total = total.Sumar(t.Valor)
		if prev, repetido := acumulados[k]; repetido {
			t.BaseImponible = t.BaseImponible.Sumar(prev.BaseImponible)
			t.Valor = t.Valor.Sumar(prev.Valor)
		}
		acumulados[k] = t
		if _, ok := bases[k]; !ok && len(porLinea) > 0 {
			v.Errorf(p+"codigo", "totales.impuesto-sin-detalle", "el impuesto %s/%s no aparece en ningún detalle", t.Codigo, t.CodigoPorcentaje)
		} else if t.Tarifa != nil && len(porLinea) > 0 && !contiene(tarifas[k], *t.Tarifa) {
			v.Errorf(p+"tarifa", "totales.tarifa", "es %s pero los detalles con el impuesto %s/%s declaran %s", *t.Tarifa, t.Codigo, t.CodigoPorcentaje, listaDecimales(tarifas[k]))
		}
	}
	for _, k := range orden {
		t, ok := acumulados[k]
		c := fmt.Sprintf("%s(%s/%s)", campo, k.codigo, k.porcentaje)
		if !ok {
			v.Errorf(c, "totales.impuesto-faltante", "los detalles declaran el impuesto %s/%s pero falta en totalConImpuestos", k.codigo, k.porcentaje)
			continue
		}
		if !t.BaseImponible.Igual(bases[k]) {
			v.Errorf(c+".baseImponible", "totales.base", "es %s pero las bases de los detalles suman %s", t.BaseImponible, bases[k].Redondear(2))
		}
		// Se acepta el impuesto calculado sobre la base acumulada (redondeado
		// al centavo más cercano) o la suma exacta de los impuestos de cada
		// línea (decisión D5). Como cada línea solo admite su propio
		// redondeo (ImporteRedondeado), los dos criterios difieren a lo sumo
		// en medio centavo por línea, y ninguna otra diferencia pasa.
		if !t.Valor.Igual(recalculado[k]) && !t.Valor.Igual(sumaLineas[k]) &&
			(k.codigo != ImpuestoIVA || len(tarifas[k]) != 1 || !ImporteRedondeado(t.Valor, exacto[k])) {
			v.Errorf(c+".valor", "totales.impuesto", "es %s; se esperaba %s (sobre la base total) o %s (suma de las líneas)",
				t.Valor, recalculado[k], sumaLineas[k].Redondear(2))
		}
	}
	return total
}

func contiene(ds []decimal.Decimal, d decimal.Decimal) bool {
	for _, x := range ds {
		if x.Igual(d) {
			return true
		}
	}
	return false
}

func listaDecimales(ds []decimal.Decimal) string {
	partes := make([]string, len(ds))
	for i, d := range ds {
		partes[i] = d.String()
	}
	return strings.Join(partes, ", ")
}

// ClaveAcceso comprueba que la clave de acceso de infoTributaria sea válida
// y que cada componente coincida con el comprobante (rechazo 58 del SRI).
func (v *Validador) ClaveAcceso(it InfoTributaria, fecha Fecha) {
	const campo = "infoTributaria.claveAcceso"
	if it.ClaveAcceso == "" {
		v.Errorf(campo, "requerido", "falta la clave de acceso; use GenerarClaveAcceso")
		return
	}
	c, err := clave.Analizar(it.ClaveAcceso)
	if err != nil {
		v.Errorf(campo, "clave.formato", "%v", err)
		return
	}
	dif := func(nombre, enClave, enDoc string) {
		if enClave != enDoc {
			v.Errorf(campo, "clave.coherencia", "%s de la clave (%s) no coincide con el comprobante (%s)", nombre, enClave, enDoc)
		}
	}
	if !fecha.EsCero() {
		dif("la fecha de emisión", c.FechaEmision.Format(formatoFecha), fecha.String())
	}
	dif("el tipo de comprobante", c.TipoComprobante, it.CodDoc)
	dif("el RUC", c.RUC, it.RUC)
	dif("el ambiente", c.Ambiente, string(it.Ambiente))
	dif("el establecimiento", c.Establecimiento, it.Estab)
	dif("el punto de emisión", c.PuntoEmision, it.PtoEmi)
	dif("el secuencial", c.Secuencial, it.Secuencial)
	dif("el tipo de emisión", c.TipoEmision, it.TipoEmision)
}

// GenerarClaveAcceso compone la clave de acceso de un comprobante a partir
// de su infoTributaria y su fecha de emisión, y la guarda en it.ClaveAcceso.
// Completa CodDoc y TipoEmision si están vacíos. Si codigoNumerico está
// vacío se genera uno aleatorio.
func GenerarClaveAcceso(it *InfoTributaria, fecha Fecha, codDoc, codigoNumerico string) (string, error) {
	if codigoNumerico == "" {
		var err error
		if codigoNumerico, err = clave.CodigoAleatorio(); err != nil {
			return "", err
		}
	}
	if it.CodDoc == "" {
		it.CodDoc = codDoc
	}
	if it.TipoEmision == "" {
		it.TipoEmision = EmisionNormal
	}
	c, err := clave.Generar(clave.Componentes{
		FechaEmision:    fecha.Time(),
		TipoComprobante: it.CodDoc,
		RUC:             it.RUC,
		Ambiente:        string(it.Ambiente),
		Establecimiento: it.Estab,
		PuntoEmision:    it.PtoEmi,
		Secuencial:      it.Secuencial,
		CodigoNumerico:  codigoNumerico,
		TipoEmision:     it.TipoEmision,
	})
	if err != nil {
		return "", err
	}
	it.ClaveAcceso = c
	return c, nil
}

// DevolucionIVA rechaza un <valorDevolucionIva> mayor que cero.
//
// El Anexo 20 exige que ese valor sea el autorizado por los servicios web
// de devolución automática del IVA (DIG) y que se reste del importe total
// del comprobante. El SDK no implementa esos servicios ni ese descuento
// (ver docs/devolucion-iva-dig.md), de modo que aceptar un valor positivo
// produciría un comprobante con el importe total mal calculado, que además
// el SRI rechazaría con el error 92. Hasta que DIG esté implementado, el
// valor solo puede omitirse o ser cero.
func (v *Validador) DevolucionIVA(campo string, valor *decimal.Decimal) {
	if valor == nil {
		return
	}
	v.Numero(campo, *valor, 14, 2)
	if valor.Signo() > 0 {
		v.Errorf(campo, "devolucion-iva.no-admitida", "la devolución automática del IVA (Anexo 20, servicios DIG) no está implementada: el SDK no descuenta este valor del importe total ni puede obtenerlo del SRI; omita el campo o use 0 (ver docs/devolucion-iva-dig.md)")
	}
}

// Compensaciones valida un bloque <compensaciones>. Una compensación es una
// rebaja de puntos porcentuales del IVA (descuento solidario de 2016-2017 y
// devolución por medios electrónicos de pago): su valor es un porcentaje
// (tarifa) de la base gravada con IVA y se resta del importe total. Como el
// SDK resta del importe lo que se declare, se acota aquí:
//
//   - compensacion.codigo: el único código que admiten los XSD es 1.
//   - compensacion.valor: el valor no puede superar base gravada × tarifa %.
//   - compensacion.total: las compensaciones no pueden sumar más que el IVA
//     del comprobante.
//   - compensacion.historica (advertencia): la figura no está vigente y no
//     se ha probado contra el SRI.
//
// totales son los impuestos totales del comprobante; campo, la ruta del
// bloque (por ejemplo "infoFactura.compensaciones").
func (v *Validador) Compensaciones(campo string, cs []Compensacion, totales []TotalImpuesto) {
	if len(cs) == 0 {
		return
	}
	gravada, iva := decimal.Cero, decimal.Cero
	for _, t := range totales {
		if t.Codigo == ImpuestoIVA && t.Valor.Signo() > 0 {
			gravada = gravada.Sumar(t.BaseImponible)
			iva = iva.Sumar(t.Valor)
		}
	}
	suma := decimal.Cero
	for i, c := range cs {
		p := fmt.Sprintf("%s[%d].", campo, i)
		if c.Codigo != "1" {
			v.Errorf(p+"codigo", "compensacion.codigo", "código %q desconocido; el esquema solo admite 1", c.Codigo)
		}
		v.Numero(p+"tarifa", c.Tarifa, 4, 2)
		v.Numero(p+"valor", c.Valor, 14, 2)
		if tope := gravada.Porcentaje(c.Tarifa); c.Valor.Restar(tope).Cmp(medioCentavo) > 0 {
			v.Errorf(p+"valor", "compensacion.valor", "es %s, más que la base gravada con IVA (%s) × %s %% = %s", c.Valor, gravada.Redondear(2), c.Tarifa, tope.Redondear(2))
		}
		suma = suma.Sumar(c.Valor)
	}
	if suma.Cmp(iva) > 0 {
		v.Errorf(campo, "compensacion.total", "las compensaciones suman %s, más que el IVA del comprobante (%s)", suma.Redondear(2), iva.Redondear(2))
	}
	v.Advertirf(campo, "compensacion.historica", "las compensaciones (descuento solidario de IVA y devolución por medios electrónicos, 2016-2017) no están vigentes y no se han probado contra el SRI")
}

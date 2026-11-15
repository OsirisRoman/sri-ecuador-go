package comprobante

import (
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// TotalLinea devuelve cantidad × precioUnitario − descuento con 2 decimales:
// el <precioTotalSinImpuesto> de un detalle.
func TotalLinea(cantidad, precioUnitario, descuento decimal.Decimal) decimal.Decimal {
	return cantidad.Multiplicar(precioUnitario).Restar(descuento).Redondear(2)
}

// CalcularImpuestos completa los impuestos de un detalle cuyo total sin
// impuestos es total, modificando el slice recibido:
//
//   - ICE e IRBPNR: se respetan la base y el valor indicados; si faltan, la
//     base es el total de la línea y el valor es base × tarifa.
//   - IVA: la tarifa se toma de la Tabla 17 cuando el código la determina;
//     la base es el total de la línea más el ICE de la misma línea, que
//     integra la base imponible del IVA.
func CalcularImpuestos(total decimal.Decimal, impuestos []Impuesto) {
	ice := decimal.Cero
	for j := range impuestos {
		imp := &impuestos[j]
		if imp.Codigo == ImpuestoIVA {
			continue
		}
		if imp.BaseImponible.EsCero() {
			imp.BaseImponible = total
		}
		imp.BaseImponible = imp.BaseImponible.Redondear(2)
		if imp.Valor.EsCero() && !imp.Tarifa.EsCero() {
			imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa)
		}
		imp.Valor = imp.Valor.Redondear(2)
		if imp.Codigo == ImpuestoICE {
			ice = ice.Sumar(imp.Valor)
		}
	}
	for j := range impuestos {
		imp := &impuestos[j]
		if imp.Codigo != ImpuestoIVA {
			continue
		}
		if t, ok := TarifasIVA[imp.CodigoPorcentaje]; ok && t.Fija {
			imp.Tarifa = t.Tarifa
		}
		imp.BaseImponible = total.Sumar(ice).Redondear(2)
		imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa).Redondear(2)
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

// toleranciaImpuesto es la diferencia admitida entre el impuesto declarado
// en una línea y base × tarifa.
var toleranciaImpuesto = decimal.MustParse("0.01")

// Numero valida las facetas de un xsd:decimal del SRI: no negativo, con un
// máximo de dígitos totales y de decimales.
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
func (v *Validador) ImpuestoLinea(p string, imp Impuesto) {
	v.Numero(p+"tarifa", imp.Tarifa, 4, 2)
	v.Numero(p+"baseImponible", imp.BaseImponible, 14, 2)
	v.Numero(p+"valor", imp.Valor, 14, 2)
	if !SoloDigitos(imp.CodigoPorcentaje) || len(imp.CodigoPorcentaje) > 4 {
		v.Errorf(p+"codigoPorcentaje", "formato", "debe tener de 1 a 4 dígitos")
	}
	switch imp.Codigo {
	case ImpuestoIVA:
		t, ok := TarifasIVA[imp.CodigoPorcentaje]
		if !ok {
			v.Errorf(p+"codigoPorcentaje", "catalogo", "código de IVA %q desconocido (Tabla 17)", imp.CodigoPorcentaje)
			return
		}
		if t.Fija && !imp.Tarifa.Igual(t.Tarifa) {
			v.Errorf(p+"tarifa", "iva.tarifa", "el código %s corresponde a %s; la tarifa declarada es %s", t.Codigo, t.Descripcion, imp.Tarifa)
		}
		esperado := imp.BaseImponible.Porcentaje(imp.Tarifa).Redondear(2)
		if imp.Valor.Restar(esperado).Abs().Cmp(toleranciaImpuesto) > 0 {
			v.Errorf(p+"valor", "iva.valor", "es %s pero base × tarifa = %s", imp.Valor, esperado)
		}
	case ImpuestoICE, ImpuestoIRBPNR:
		// Tarifas específicas o mixtas: no se recalculan aquí.
	default:
		v.Errorf(p+"codigo", "catalogo", "código de impuesto %q desconocido (Tabla 16: 2 IVA, 3 ICE, 5 IRBPNR)", imp.Codigo)
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
	sumaLineas := map[par]decimal.Decimal{}
	var orden []par
	for _, g := range AgruparImpuestos(porLinea...) {
		k := par{g.Codigo, g.CodigoPorcentaje}
		if _, visto := bases[k]; !visto {
			orden = append(orden, k)
		}
		bases[k] = bases[k].Sumar(g.Base)
		recalculado[k] = recalculado[k].Sumar(g.Valor())
		sumaLineas[k] = sumaLineas[k].Sumar(g.SumaLineas)
	}
	acumulados := map[par]TotalImpuesto{}
	total := decimal.Cero
	for i, t := range declarados {
		p := fmt.Sprintf("%s[%d].", campo, i)
		k := par{t.Codigo, t.CodigoPorcentaje}
		v.Numero(p+"baseImponible", t.BaseImponible, 14, 2)
		v.Numero(p+"valor", t.Valor, 14, 2)
		total = total.Sumar(t.Valor)
		if prev, repetido := acumulados[k]; repetido {
			t.BaseImponible = t.BaseImponible.Sumar(prev.BaseImponible)
			t.Valor = t.Valor.Sumar(prev.Valor)
		}
		acumulados[k] = t
		if _, ok := bases[k]; !ok && len(porLinea) > 0 {
			v.Errorf(p+"codigo", "totales.impuesto-sin-detalle", "el impuesto %s/%s no aparece en ningún detalle", t.Codigo, t.CodigoPorcentaje)
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
		// Se acepta tanto el impuesto calculado sobre la base acumulada
		// como la suma de los impuestos de cada línea: ambos criterios son
		// de uso corriente y difieren, como mucho, en centavos de redondeo.
		if !t.Valor.Igual(recalculado[k]) && !t.Valor.Igual(sumaLineas[k]) {
			v.Errorf(c+".valor", "totales.impuesto", "es %s; se esperaba %s (sobre la base total) o %s (suma de las líneas)",
				t.Valor, recalculado[k], sumaLineas[k].Redondear(2))
		}
	}
	return total
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

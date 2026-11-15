package factura

import (
	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// claveGrupo agrupa impuestos para <totalConImpuestos>. La tarifa forma
// parte de la clave porque el IVA diferenciado (código 8) puede aparecer con
// tarifas distintas bajo el mismo código.
type claveGrupo struct {
	codigo, porcentaje, tarifa string
}

type grupo struct {
	clave  claveGrupo
	tarifa decimal.Decimal
	base   decimal.Decimal
	// sumaLineas es la suma de los valores de impuesto ya redondeados de
	// cada línea.
	sumaLineas decimal.Decimal
}

// totalLinea devuelve cantidad × precioUnitario − descuento con 2 decimales.
func totalLinea(d Detalle) decimal.Decimal {
	return d.Cantidad.Multiplicar(d.PrecioUnitario).Restar(d.Descuento).Redondear(2)
}

// agrupar recorre los impuestos de los detalles en orden de aparición.
func (f *Factura) agrupar() []*grupo {
	var orden []*grupo
	indice := map[claveGrupo]*grupo{}
	for _, d := range f.Detalles {
		for _, imp := range d.Impuestos {
			k := claveGrupo{imp.Codigo, imp.CodigoPorcentaje, imp.Tarifa.Redondear(2).String()}
			g, ok := indice[k]
			if !ok {
				g = &grupo{clave: k, tarifa: imp.Tarifa}
				indice[k] = g
				orden = append(orden, g)
			}
			g.base = g.base.Sumar(imp.BaseImponible)
			g.sumaLineas = g.sumaLineas.Sumar(imp.Valor)
		}
	}
	return orden
}

// valorGrupo es el impuesto total de un grupo. Para el IVA se calcula sobre
// la base acumulada (base × tarifa), que es como se liquida el impuesto del
// comprobante; para ICE e IRBPNR, que pueden tener tarifas específicas por
// unidad, se suman los valores de las líneas.
func (g *grupo) valor() decimal.Decimal {
	if g.clave.codigo == comprobante.ImpuestoIVA {
		return g.base.Porcentaje(g.tarifa).Redondear(2)
	}
	return g.sumaLineas.Redondear(2)
}

// Calcular completa los importes derivados a partir de cantidad, precio
// unitario, descuento y códigos de impuesto de cada detalle:
//
//   - Detalle: precioTotalSinImpuesto = cantidad × precioUnitario − descuento.
//   - Impuestos del detalle: para el IVA, la tarifa según la Tabla 17, la
//     base (el total de la línea más el ICE de la misma línea, que integra
//     la base del IVA) y el valor. Para ICE e IRBPNR se respetan la base y
//     el valor indicados; si faltan, se toma como base el total de la línea
//     y se aplica la tarifa como porcentaje.
//   - infoFactura: totalSinImpuestos, totalDescuento, totalConImpuestos e
//     importeTotal (incluye propina, rubros de exportación y de terceros).
//   - Pagos: si hay un único pago sin total, se le asigna el importe total.
//
// Todos los importes monetarios se redondean a 2 decimales, mitad hacia
// arriba. Calcular es idempotente y no modifica los datos de entrada
// (cantidades, precios, descuentos, códigos).
func (f *Factura) Calcular() {
	totalSin, totalDesc := decimal.Cero, decimal.Cero
	for i := range f.Detalles {
		d := &f.Detalles[i]
		d.Descuento = d.Descuento.Redondear(2)
		d.PrecioTotalSinImpuesto = totalLinea(*d)
		totalSin = totalSin.Sumar(d.PrecioTotalSinImpuesto)
		totalDesc = totalDesc.Sumar(d.Descuento)

		// Primero ICE/IRBPNR: el ICE forma parte de la base del IVA.
		ice := decimal.Cero
		for j := range d.Impuestos {
			imp := &d.Impuestos[j]
			if imp.Codigo == comprobante.ImpuestoIVA {
				continue
			}
			if imp.BaseImponible.EsCero() {
				imp.BaseImponible = d.PrecioTotalSinImpuesto
			}
			imp.BaseImponible = imp.BaseImponible.Redondear(2)
			if imp.Valor.EsCero() && !imp.Tarifa.EsCero() {
				imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa)
			}
			imp.Valor = imp.Valor.Redondear(2)
			if imp.Codigo == comprobante.ImpuestoICE {
				ice = ice.Sumar(imp.Valor)
			}
		}
		for j := range d.Impuestos {
			imp := &d.Impuestos[j]
			if imp.Codigo != comprobante.ImpuestoIVA {
				continue
			}
			if t, ok := comprobante.TarifasIVA[imp.CodigoPorcentaje]; ok && t.Fija {
				imp.Tarifa = t.Tarifa
			}
			imp.BaseImponible = d.PrecioTotalSinImpuesto.Sumar(ice).Redondear(2)
			imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa).Redondear(2)
		}
	}

	info := &f.InfoFactura
	info.TotalSinImpuestos = totalSin.Redondear(2)
	info.TotalDescuento = totalDesc.Redondear(2)

	// Se conservan los datos que el usuario haya puesto en un total y que
	// no se derivan de los detalles.
	previos := map[claveGrupo]comprobante.TotalImpuesto{}
	for _, t := range info.TotalConImpuestos {
		previos[claveGrupo{codigo: t.Codigo, porcentaje: t.CodigoPorcentaje}] = t
	}
	info.TotalConImpuestos = nil
	impuestos := decimal.Cero
	for _, g := range f.agrupar() {
		tarifa := g.tarifa.Redondear(2)
		t := comprobante.TotalImpuesto{
			Codigo:           g.clave.codigo,
			CodigoPorcentaje: g.clave.porcentaje,
			BaseImponible:    g.base.Redondear(2),
			Tarifa:           &tarifa,
			Valor:            g.valor(),
		}
		if p, ok := previos[claveGrupo{codigo: g.clave.codigo, porcentaje: g.clave.porcentaje}]; ok {
			t.DescuentoAdicional = p.DescuentoAdicional
			t.ValorDevolucionIva = p.ValorDevolucionIva
		}
		info.TotalConImpuestos = append(info.TotalConImpuestos, t)
		impuestos = impuestos.Sumar(t.Valor)
	}

	info.Propina = info.Propina.Redondear(2)
	info.ImporteTotal = f.importeEsperado(impuestos)
	if len(info.Pagos) == 1 && info.Pagos[0].Total.EsCero() {
		info.Pagos[0].Total = info.ImporteTotal
	}
	if info.Moneda == "" {
		info.Moneda = comprobante.MonedaDolar
	}
}

// importeEsperado aplica la fórmula del importe total a partir de la suma
// de impuestos.
func (f *Factura) importeEsperado(impuestos decimal.Decimal) decimal.Decimal {
	info := &f.InfoFactura
	total := info.TotalSinImpuestos.Sumar(impuestos).Sumar(info.Propina)
	for _, extra := range []*decimal.Decimal{info.FleteInternacional, info.SeguroInternacional, info.GastosAduaneros, info.GastosTransporteOtros} {
		if extra != nil {
			total = total.Sumar(*extra)
		}
	}
	for _, r := range f.OtrosRubrosTerceros {
		total = total.Sumar(r.Total)
	}
	for _, c := range info.Compensaciones {
		total = total.Restar(c.Valor)
	}
	return total.Redondear(2)
}

// GenerarClaveAcceso compone la clave de acceso a partir de infoTributaria
// y de la fecha de emisión, y la guarda en InfoTributaria.ClaveAcceso.
//
// codigoNumerico son los 8 dígitos a discreción del emisor; si está vacío
// se genera uno aleatorio. La clave identifica al comprobante durante toda
// su vida: guárdela, porque un comprobante devuelto o no autorizado debe
// reenviarse con la misma clave (ficha técnica 5.10).
func (f *Factura) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	if codigoNumerico == "" {
		var err error
		if codigoNumerico, err = clave.CodigoAleatorio(); err != nil {
			return "", err
		}
	}
	it := &f.InfoTributaria
	if it.CodDoc == "" {
		it.CodDoc = comprobante.CodFactura
	}
	if it.TipoEmision == "" {
		it.TipoEmision = comprobante.EmisionNormal
	}
	c, err := clave.Generar(clave.Componentes{
		FechaEmision:    f.InfoFactura.FechaEmision.Time(),
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

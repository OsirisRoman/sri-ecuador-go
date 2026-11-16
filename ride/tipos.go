package ride

import (
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/guiaremision"
	"github.com/OsirisRoman/sri-ecuador-go/liquidacion"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
)

func detAdicionales(das []comprobante.DetAdicional) string {
	var ls []string
	for _, da := range das {
		ls = append(ls, da.Nombre+": "+da.Valor)
	}
	return strings.Join(ls, "\n")
}

// nombreTarifa describe un código de IVA de la Tabla 17: "15%", "0%",
// "No objeto de impuesto"…
func nombreTarifa(codigo string, tarifa *decimal.Decimal) string {
	if t, ok := comprobante.TarifaIVADe(codigo); ok && t.Fija {
		return strings.ToUpper(t.Descripcion)
	}
	if tarifa != nil {
		return tarifa.String() + "%"
	}
	return "CÓDIGO " + codigo
}

// totalesImpuestos compone las filas de subtotales e impuestos comunes a
// factura, liquidación y notas: un subtotal por cada tarifa de IVA
// declarada (ficha, Anexo 2: «solo los subtotales que fueron llenados»),
// el subtotal sin impuestos, el descuento y el valor de cada impuesto.
func totalesImpuestos(tis []comprobante.TotalImpuesto, sinImpuestos decimal.Decimal, descuento *decimal.Decimal) []par {
	var ps, valores []par
	for _, t := range tis {
		switch t.Codigo {
		case comprobante.ImpuestoIVA:
			nombre := nombreTarifa(t.CodigoPorcentaje, t.Tarifa)
			ps = append(ps, par{"SUBTOTAL " + nombre, t.BaseImponible.String()})
			if strings.HasSuffix(nombre, "%") && nombre != "0%" {
				valores = append(valores, par{"IVA " + nombre, t.Valor.String()})
			}
		case comprobante.ImpuestoICE:
			valores = append(valores, par{"ICE", t.Valor.String()})
		case comprobante.ImpuestoIRBPNR:
			valores = append(valores, par{"IRBPNR", t.Valor.String()})
		default:
			valores = append(valores, par{"IMPUESTO " + t.Codigo, t.Valor.String()})
		}
	}
	ps = append(ps, par{"SUBTOTAL SIN IMPUESTOS", sinImpuestos.String()})
	if descuento != nil {
		ps = append(ps, par{"TOTAL DESCUENTO", descuento.String()})
	}
	return append(ps, valores...)
}

var columnasVenta = []columna{
	{"Cod. Principal", 10, izquierda}, {"Cod. Auxiliar", 9, izquierda}, {"Cantidad", 9, derecha},
	{"Descripción", 24, izquierda}, {"Detalle Adicional", 15, izquierda},
	{"Precio Unitario", 11, derecha}, {"Descuento", 10, derecha}, {"Precio Total", 12, derecha},
}

func filasVenta(ds []factura.Detalle) [][]string {
	var filas [][]string
	for _, d := range ds {
		filas = append(filas, []string{d.CodigoPrincipal, d.CodigoAuxiliar, d.Cantidad.String(), d.Descripcion,
			detAdicionales(d.DetallesAdicionales), d.PrecioUnitario.String(), d.Descuento.String(), d.PrecioTotalSinImpuesto.String()})
	}
	return filas
}

func opcional(ps []par, rotulo string, v *decimal.Decimal) []par {
	if v != nil && !v.EsCero() {
		ps = append(ps, par{rotulo, v.String()})
	}
	return ps
}

func deFactura(doc []byte) (*modelo, error) {
	f, err := factura.Parse(doc)
	if err != nil {
		return nil, err
	}
	i := f.InfoFactura
	receptor := [][]par{
		{{"Razón Social / Nombres y Apellidos", i.RazonSocialComprador}},
		{{"Identificación", i.IdentificacionComprador}, {"Fecha Emisión", i.FechaEmision.String()}},
	}
	if i.DireccionComprador != "" {
		receptor = append(receptor, []par{{"Dirección", i.DireccionComprador}})
	}
	if i.GuiaRemision != "" || i.Placa != "" {
		receptor = append(receptor, []par{{"Guía Remisión", i.GuiaRemision}, {"Placa / Matrícula", i.Placa}})
	}
	tot := totalesImpuestos(i.TotalConImpuestos, i.TotalSinImpuestos, &i.TotalDescuento)
	if !i.Propina.EsCero() {
		tot = append(tot, par{"PROPINA", i.Propina.String()})
	}
	tot = opcional(tot, "FLETE INTERNACIONAL", i.FleteInternacional)
	tot = opcional(tot, "SEGURO INTERNACIONAL", i.SeguroInternacional)
	tot = opcional(tot, "GASTOS ADUANEROS", i.GastosAduaneros)
	tot = opcional(tot, "GASTOS DE TRANSPORTE OTROS", i.GastosTransporteOtros)
	for _, r := range f.OtrosRubrosTerceros {
		tot = append(tot, par{strings.ToUpper(r.Concepto), r.Total.String()})
	}
	tot = append(tot, par{rotuloTotal, i.ImporteTotal.String()})
	tot = opcional(tot, "VALOR RETENCIÓN IVA", i.ValorRetIva)
	tot = opcional(tot, "VALOR RETENCIÓN RENTA", i.ValorRetRenta)
	m := &modelo{
		titulo: "FACTURA", it: f.InfoTributaria,
		nombre: "Factura", receptor: i.RazonSocialComprador,
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: f.InfoAdicional, pagos: i.Pagos, totales: tot,
	}
	m.bloques = append(m.bloques,
		func(l *lienzo) { l.caja(receptor) },
		func(l *lienzo) { l.tabla(margen, anchoUtil, columnasVenta, filasVenta(f.Detalles)); l.y += separa })
	if len(f.Retenciones) > 0 {
		var filas [][]string
		for _, r := range f.Retenciones {
			filas = append(filas, []string{r.Codigo, r.CodigoPorcentaje, r.Tarifa.String(), r.Valor.String()})
		}
		m.bloques = append(m.bloques, func(l *lienzo) {
			l.tabla(margen, anchoUtil*0.56, []columna{{"Retención (código)", 3, centro}, {"Código porcentaje", 3, centro}, {"Tarifa", 2, derecha}, {"Valor", 2, derecha}}, filas)
			l.y += separa
		})
	}
	return m, nil
}

func deLiquidacion(doc []byte) (*modelo, error) {
	q, err := liquidacion.Parse(doc)
	if err != nil {
		return nil, err
	}
	i := q.InfoLiquidacionCompra
	proveedor := [][]par{
		{{"Razón Social / Nombres y Apellidos (Proveedor)", i.RazonSocialProveedor}},
		{{"Identificación", i.IdentificacionProveedor}, {"Fecha Emisión", i.FechaEmision.String()}},
	}
	if i.DireccionProveedor != "" {
		proveedor = append(proveedor, []par{{"Dirección", i.DireccionProveedor}})
	}
	tot := totalesImpuestos(i.TotalConImpuestos, i.TotalSinImpuestos, &i.TotalDescuento)
	tot = append(tot, par{rotuloTotal, i.ImporteTotal.String()})
	tot = opcional(tot, "TOTAL COMPROBANTES DE REEMBOLSO", i.TotalComprobantesReembolso)
	return &modelo{
		titulo: "LIQUIDACIÓN DE COMPRA DE BIENES Y PRESTACIÓN DE SERVICIOS", it: q.InfoTributaria,
		nombre: "Liquidación de compra", receptor: i.RazonSocialProveedor,
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: q.InfoAdicional, pagos: i.Pagos, totales: tot,
		bloques: []func(*lienzo){
			func(l *lienzo) { l.caja(proveedor) },
			func(l *lienzo) { l.tabla(margen, anchoUtil, columnasVenta, filasVenta(q.Detalles)); l.y += separa },
		},
	}, nil
}

func deNotaCredito(doc []byte) (*modelo, error) {
	n, err := notacredito.Parse(doc)
	if err != nil {
		return nil, err
	}
	i := n.InfoNotaCredito
	receptor := [][]par{
		{{"Razón Social / Nombres y Apellidos", i.RazonSocialComprador}},
		{{"Identificación", i.IdentificacionComprador}, {"Fecha Emisión", i.FechaEmision.String()}},
		{{"Comprobante que se modifica", strings.TrimSpace(nombreDocumento(i.CodDocModificado) + " " + i.NumDocModificado)}},
		{{"Fecha Emisión (Comprobante a modificar)", i.FechaEmisionDocSustento.String()}},
		{{"Razón de Modificación", i.Motivo}},
	}
	var filas [][]string
	for _, d := range n.Detalles {
		filas = append(filas, []string{d.CodigoInterno, d.CodigoAdicional, d.Cantidad.String(), d.Descripcion,
			detAdicionales(d.DetallesAdicionales), d.PrecioUnitario.String(), d.Descuento.String(), d.PrecioTotalSinImpuesto.String()})
	}
	tot := totalesImpuestos(i.TotalConImpuestos, i.TotalSinImpuestos, nil)
	tot = append(tot, par{rotuloTotal, i.ValorModificacion.String()})
	return &modelo{
		titulo: "NOTA DE CRÉDITO", it: n.InfoTributaria,
		nombre: "Nota de crédito", receptor: i.RazonSocialComprador,
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: n.InfoAdicional, totales: tot,
		bloques: []func(*lienzo){
			func(l *lienzo) { l.caja(receptor) },
			func(l *lienzo) { l.tabla(margen, anchoUtil, columnasVenta, filas); l.y += separa },
		},
	}, nil
}

func deNotaDebito(doc []byte) (*modelo, error) {
	n, err := notadebito.Parse(doc)
	if err != nil {
		return nil, err
	}
	i := n.InfoNotaDebito
	receptor := [][]par{
		{{"Razón Social / Nombres y Apellidos", i.RazonSocialComprador}},
		{{"Identificación", i.IdentificacionComprador}, {"Fecha Emisión", i.FechaEmision.String()}},
		{{"Comprobante que se modifica", strings.TrimSpace(nombreDocumento(i.CodDocModificado) + " " + i.NumDocModificado)}},
		{{"Fecha Emisión (Comprobante a modificar)", i.FechaEmisionDocSustento.String()}},
	}
	var filas [][]string
	for _, mo := range n.Motivos {
		filas = append(filas, []string{mo.Razon, mo.Valor.String()})
	}
	var tis []comprobante.TotalImpuesto
	for _, imp := range i.Impuestos {
		tarifa := imp.Tarifa
		tis = append(tis, comprobante.TotalImpuesto{Codigo: imp.Codigo, CodigoPorcentaje: imp.CodigoPorcentaje,
			BaseImponible: imp.BaseImponible, Tarifa: &tarifa, Valor: imp.Valor})
	}
	tot := totalesImpuestos(tis, i.TotalSinImpuestos, nil)
	tot = append(tot, par{rotuloTotal, i.ValorTotal.String()})
	return &modelo{
		titulo: "NOTA DE DÉBITO", it: n.InfoTributaria,
		nombre: "Nota de débito", receptor: i.RazonSocialComprador,
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: n.InfoAdicional, pagos: i.Pagos, totales: tot,
		bloques: []func(*lienzo){
			func(l *lienzo) { l.caja(receptor) },
			func(l *lienzo) {
				l.tabla(margen, anchoUtil, []columna{{"RAZÓN DE LA MODIFICACIÓN", 75, izquierda}, {"VALOR DE LA MODIFICACIÓN", 25, derecha}}, filas)
				l.y += separa
			},
		},
	}, nil
}

// numeroConGuiones presenta un número de comprobante de 15 dígitos como
// 001-001-000000001.
func numeroConGuiones(n string) string {
	if len(n) == 15 && comprobante.SoloDigitos(n) {
		return n[:3] + "-" + n[3:6] + "-" + n[6:]
	}
	return n
}

func nombreImpuestoRetenido(codigo string) string {
	switch codigo {
	case retencion.ImpuestoRenta:
		return "RENTA"
	case retencion.ImpuestoIVA:
		return "IVA"
	case retencion.ImpuestoISD:
		return "ISD"
	}
	return codigo
}

var columnasRetencion = []columna{
	{"Comprobante", 12, izquierda}, {"Número", 18, izquierda}, {"Fecha Emisión", 11, centro}, {"Ejercicio Fiscal", 9, centro},
	{"Base Imponible para la Retención", 12, derecha}, {"Impuesto", 9, centro}, {"Código", 8, centro},
	{"Porcentaje Retención", 10, derecha}, {"Valor Retenido", 11, derecha},
}

func deRetencionV1(doc []byte) (*modelo, error) {
	c, err := retencion.ParseV1(doc)
	if err != nil {
		return nil, err
	}
	i := c.InfoCompRetencion
	sujeto := [][]par{
		{{"Razón Social / Nombres y Apellidos", i.RazonSocialSujetoRetenido}},
		{{"Identificación", i.IdentificacionSujetoRetenido}, {"Fecha Emisión", i.FechaEmision.String()}},
	}
	var filas [][]string
	for _, r := range c.Impuestos {
		fecha := ""
		if r.FechaEmisionDocSustento != nil {
			fecha = r.FechaEmisionDocSustento.String()
		}
		filas = append(filas, []string{nombreDocumento(r.CodDocSustento), numeroConGuiones(r.NumDocSustento), fecha, i.PeriodoFiscal,
			r.BaseImponible.String(), nombreImpuestoRetenido(r.Codigo), r.CodigoRetencion, r.PorcentajeRetener.String(), r.ValorRetenido.String()})
	}
	return &modelo{
		titulo: "COMPROBANTE DE RETENCIÓN", it: c.InfoTributaria,
		nombre: "Comprobante de retención", receptor: i.RazonSocialSujetoRetenido,
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: c.InfoAdicional, totales: []par{{"TOTAL RETENIDO", c.TotalRetenido().String()}},
		bloques: []func(*lienzo){
			func(l *lienzo) { l.caja(sujeto) },
			func(l *lienzo) { l.tabla(margen, anchoUtil, columnasRetencion, filas); l.y += separa },
		},
	}, nil
}

func deRetencion(doc []byte) (*modelo, error) {
	c, err := retencion.Parse(doc)
	if err != nil {
		return nil, err
	}
	i := c.InfoCompRetencion
	sujeto := [][]par{
		{{"Razón Social / Nombres y Apellidos", i.RazonSocialSujetoRetenido}},
		{{"Identificación", i.IdentificacionSujetoRetenido}, {"Fecha Emisión", i.FechaEmision.String()}},
	}
	var filas [][]string
	total := decimal.Cero
	for _, ds := range c.DocsSustento {
		for _, r := range ds.Retenciones {
			filas = append(filas, []string{nombreDocumento(ds.CodDocSustento), numeroConGuiones(ds.NumDocSustento), ds.FechaEmisionDocSustento.String(),
				i.PeriodoFiscal, r.BaseImponible.String(), nombreImpuestoRetenido(r.Codigo), r.CodigoRetencion,
				r.PorcentajeRetener.String(), r.ValorRetenido.String()})
			total = total.Sumar(r.ValorRetenido)
		}
	}
	return &modelo{
		titulo: "COMPROBANTE DE RETENCIÓN", it: c.InfoTributaria,
		nombre: "Comprobante de retención", receptor: i.RazonSocialSujetoRetenido,
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: c.InfoAdicional, totales: []par{{"TOTAL RETENIDO", total.Redondear(2).String()}},
		bloques: []func(*lienzo){
			func(l *lienzo) { l.caja(sujeto) },
			func(l *lienzo) { l.tabla(margen, anchoUtil, columnasRetencion, filas); l.y += separa },
		},
	}, nil
}

func deGuia(doc []byte) (*modelo, error) {
	g, err := guiaremision.Parse(doc)
	if err != nil {
		return nil, err
	}
	i := g.InfoGuiaRemision
	m := &modelo{
		titulo: "GUÍA DE REMISIÓN", it: g.InfoTributaria,
		nombre: "Guía de remisión", receptor: "",
		dirEstablecimiento: i.DirEstablecimiento, contribuyenteEspecial: i.ContribuyenteEspecial, obligado: i.ObligadoContabilidad,
		adicional: g.InfoAdicional,
	}
	transporte := [][]par{
		{{"Identificación (Transportista)", i.RucTransportista}},
		{{"Razón Social / Nombres y Apellidos", i.RazonSocialTransportista}},
		{{"Placa", i.Placa}},
		{{"Punto de Partida", i.DirPartida}},
		{{"Fecha inicio Transporte", i.FechaIniTransporte.String()}, {"Fecha fin Transporte", i.FechaFinTransporte.String()}},
	}
	m.bloques = append(m.bloques, func(l *lienzo) { l.caja(transporte) })
	cols := []columna{{"Cantidad", 12, derecha}, {"Descripción", 48, izquierda}, {"Código Principal", 20, izquierda}, {"Código Auxiliar", 20, izquierda}}
	for n, d := range g.Destinatarios {
		if n == 0 {
			m.receptor = d.RazonSocialDestinatario
		}
		var datos [][]par
		if d.NumDocSustento != "" || d.CodDocSustento != "" {
			fecha := ""
			if d.FechaEmisionDocSustento != nil {
				fecha = d.FechaEmisionDocSustento.String()
			}
			datos = append(datos,
				[]par{{"Comprobante de Venta", strings.TrimSpace(nombreDocumento(d.CodDocSustento) + " " + d.NumDocSustento)}, {"Fecha de Emisión", fecha}},
				[]par{{"Número de Autorización", d.NumAutDocSustento}})
		}
		datos = append(datos,
			[]par{{"Motivo Traslado", d.MotivoTraslado}},
			[]par{{"Destino (Punto de llegada)", d.DirDestinatario}},
			[]par{{"Identificación (Destinatario)", d.IdentificacionDestinatario}},
			[]par{{"Razón Social / Nombres y Apellidos", d.RazonSocialDestinatario}})
		if d.DocAduaneroUnico != "" {
			datos = append(datos, []par{{"Documento Aduanero", d.DocAduaneroUnico}})
		}
		if d.CodEstabDestino != "" {
			datos = append(datos, []par{{"Código Establecimiento Destino", d.CodEstabDestino}})
		}
		if d.Ruta != "" {
			datos = append(datos, []par{{"Ruta", d.Ruta}})
		}
		var filas [][]string
		for _, det := range d.Detalles {
			desc := det.Descripcion
			if ad := detAdicionales(det.DetallesAdicionales); ad != "" {
				desc += "\n" + ad
			}
			filas = append(filas, []string{det.Cantidad.String(), desc, det.CodigoInterno, det.CodigoAdicional})
		}
		m.bloques = append(m.bloques, func(l *lienzo) {
			l.caja(datos)
			l.y -= separa // la tabla va pegada a su destinatario
			l.tabla(margen, anchoUtil, cols, filas)
			l.y += separa
		})
	}
	return m, nil
}

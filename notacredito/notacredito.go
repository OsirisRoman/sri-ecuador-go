// Package notacredito modela la nota de crédito electrónica del SRI
// (codDoc 04) en sus versiones 1.0.0 y 1.1.0: estructura tipada,
// serialización y lectura de XML, cálculo de totales y validación.
//
// Una nota de crédito anula o reduce un comprobante de venta ya emitido,
// que se identifica en codDocModificado, numDocModificado y
// fechaEmisionDocSustento. DesdeFactura construye la que revierte por
// completo una factura.
//
// Referencia: ficha técnica v2.34, Anexo 1 y Anexo 3 (formato XML nota de
// crédito, versiones 1.0.0 y 1.1.0).
package notacredito

import (
	"encoding/xml"
	"fmt"
	"regexp"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// Versiones del esquema de nota de crédito. La 1.1.0 admite 6 decimales en
// cantidad y precio unitario; la 1.0.0, 2.
const (
	V100              = "1.0.0"
	V110              = "1.1.0"
	VersionPorDefecto = V110
)

// NotaCredito es el comprobante <notaCredito>.
type NotaCredito struct {
	XMLName xml.Name `xml:"notaCredito"`
	ID      string   `xml:"id,attr"`
	Version string   `xml:"version,attr"`

	InfoTributaria  comprobante.InfoTributaria                    `xml:"infoTributaria"`
	InfoNotaCredito InfoNotaCredito                               `xml:"infoNotaCredito"`
	Detalles        comprobante.Lista[Detalle]                    `xml:"detalles"`
	MaquinaFiscal   *comprobante.MaquinaFiscal                    `xml:"maquinaFiscal,omitempty"`
	InfoAdicional   comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`

	// origen es la factura modificada, si la aplicación la asoció
	// (AsociarFactura o DesdeFactura). No forma parte del XML.
	origen *origen
}

// InfoNotaCredito es el bloque <infoNotaCredito>.
type InfoNotaCredito struct {
	FechaEmision       comprobante.Fecha `xml:"fechaEmision"`
	DirEstablecimiento string            `xml:"dirEstablecimiento,omitempty"`
	// El receptor debe estar identificado: no se admite consumidor final
	// (Tabla 6).
	TipoIdentificacionComprador string `xml:"tipoIdentificacionComprador"`
	RazonSocialComprador        string `xml:"razonSocialComprador"`
	IdentificacionComprador     string `xml:"identificacionComprador"`
	ContribuyenteEspecial       string `xml:"contribuyenteEspecial,omitempty"`
	ObligadoContabilidad        string `xml:"obligadoContabilidad,omitempty"`
	Rise                        string `xml:"rise,omitempty"`
	// CodDocModificado es el tipo del comprobante que se modifica
	// (comprobante.CodFactura para una factura).
	CodDocModificado string `xml:"codDocModificado"`
	// NumDocModificado tiene el formato 001-001-000000001.
	NumDocModificado        string            `xml:"numDocModificado"`
	FechaEmisionDocSustento comprobante.Fecha `xml:"fechaEmisionDocSustento"`

	TotalSinImpuestos decimal.Decimal                             `xml:"totalSinImpuestos"`
	Compensaciones    comprobante.Lista[comprobante.Compensacion] `xml:"compensaciones,omitempty"`
	// ValorModificacion es el valor total de la nota de crédito, impuestos
	// incluidos.
	ValorModificacion decimal.Decimal `xml:"valorModificacion"`
	Moneda            string          `xml:"moneda,omitempty"`
	// TotalConImpuestos: en la nota de crédito cada total no lleva tarifa
	// ni descuento adicional (el XSD no los admite).
	TotalConImpuestos comprobante.Lista[comprobante.TotalImpuesto] `xml:"totalConImpuestos"`
	// Motivo de la nota de crédito (obligatorio).
	Motivo string `xml:"motivo"`
}

// Detalle es una línea <detalle> de la nota de crédito. A diferencia de la
// factura, los códigos se llaman codigoInterno y codigoAdicional.
type Detalle struct {
	CodigoInterno   string          `xml:"codigoInterno,omitempty"`
	CodigoAdicional string          `xml:"codigoAdicional,omitempty"`
	Descripcion     string          `xml:"descripcion"`
	Cantidad        decimal.Decimal `xml:"cantidad"`
	// PrecioUnitario es el precio por unidad sin impuestos. Si se indica
	// PrecioFinal, lo deduce Calcular.
	PrecioUnitario decimal.Decimal `xml:"precioUnitario"`
	// PrecioFinal, si no es cero, declara que el detalle se pactó por su
	// precio final por unidad, con el IVA incluido; ver
	// factura.Detalle.PrecioFinal. No forma parte del XML.
	PrecioFinal            decimal.Decimal                             `xml:"-"`
	Descuento              decimal.Decimal                             `xml:"descuento"`
	PrecioTotalSinImpuesto decimal.Decimal                             `xml:"precioTotalSinImpuesto"`
	DetallesAdicionales    comprobante.Lista[comprobante.DetAdicional] `xml:"detallesAdicionales,omitempty"`
	Impuestos              comprobante.Lista[comprobante.Impuesto]     `xml:"impuestos"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Detalle) EtiquetaXML() string { return "detalle" }

func (n *NotaCredito) version() string {
	if n.Version == "" {
		return VersionPorDefecto
	}
	return n.Version
}

// Info devuelve el bloque infoTributaria.
func (n *NotaCredito) Info() comprobante.InfoTributaria { return n.InfoTributaria }

// XML serializa la nota de crédito como documento XML UTF-8 listo para
// firmar.
func (n *NotaCredito) XML() ([]byte, error) {
	copia := *n
	copia.XMLName = xml.Name{Local: "notaCredito"}
	copia.ID = "comprobante"
	copia.Version = n.version()
	return comprobante.Serializar(&copia)
}

// Parse lee una nota de crédito desde su XML, firmado o no.
func Parse(doc []byte) (*NotaCredito, error) {
	var n NotaCredito
	if err := comprobante.Deserializar(doc, &n); err != nil {
		return nil, fmt.Errorf("notacredito: XML ilegible: %w", err)
	}
	return &n, nil
}

// LineaIVA construye un detalle con un único impuesto IVA.
func LineaIVA(codigo, descripcion string, cantidad, precioUnitario decimal.Decimal, codigoIVA string) Detalle {
	return Detalle{
		CodigoInterno: codigo, Descripcion: descripcion, Cantidad: cantidad, PrecioUnitario: precioUnitario,
		Impuestos: comprobante.Lista[comprobante.Impuesto]{{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: codigoIVA}},
	}
}

// DesdeFactura construye la nota de crédito que revierte por completo una
// factura: mismo receptor, mismos detalles e impuestos, y la referencia al
// comprobante modificado. Quedan por asignar el secuencial
// (InfoTributaria.Secuencial), la fecha de emisión si no es hoy, y la clave
// de acceso (GenerarClaveAcceso). Para una devolución parcial, ajuste los
// Detalles y llame a Calcular: el IVA y el ICE que se dedujo del total de
// cada línea se recalculan; un ICE específico o con base propia, y un
// IRBPNR, se copian de la factura tal cual y debe ajustarlos la aplicación.
//
// Los detalles que la factura define por su precio final (PrecioFinal) se
// copian con él, y la nota de crédito suma lo mismo que la factura. Una
// factura leída de XML no lo conserva: si se emitió con precios finales,
// llame antes a f.DeclararPreciosFinales(), o la nota de crédito se
// calculará con precios sin impuestos y podrá diferir en un centavo por
// línea (Validar lo advierte con origen.precio-final).
//
// La nota de crédito queda asociada a f (ver AsociarFactura): Validar
// comprueba que no devuelva más de lo facturado. Si sobre la misma factura
// ya se emitieron otras notas de crédito, páselas con AsociarFactura.
func DesdeFactura(f *factura.Factura, motivo string) *NotaCredito {
	it := f.InfoTributaria
	it.CodDoc = comprobante.CodNotaCredito
	it.Secuencial = ""
	it.ClaveAcceso = ""
	n := &NotaCredito{
		InfoTributaria: it,
		InfoNotaCredito: InfoNotaCredito{
			FechaEmision:                comprobante.Hoy(),
			DirEstablecimiento:          f.InfoFactura.DirEstablecimiento,
			TipoIdentificacionComprador: f.InfoFactura.TipoIdentificacionComprador,
			RazonSocialComprador:        f.InfoFactura.RazonSocialComprador,
			IdentificacionComprador:     f.InfoFactura.IdentificacionComprador,
			ContribuyenteEspecial:       f.InfoFactura.ContribuyenteEspecial,
			ObligadoContabilidad:        f.InfoFactura.ObligadoContabilidad,
			CodDocModificado:            comprobante.CodFactura,
			NumDocModificado:            f.InfoTributaria.NumeroComprobante(),
			FechaEmisionDocSustento:     f.InfoFactura.FechaEmision,
			Moneda:                      f.InfoFactura.Moneda,
			Motivo:                      motivo,
		},
	}
	for _, d := range f.Detalles {
		n.Detalles = append(n.Detalles, Detalle{
			CodigoInterno: d.CodigoPrincipal, CodigoAdicional: d.CodigoAuxiliar, Descripcion: d.Descripcion,
			Cantidad: d.Cantidad, PrecioUnitario: d.PrecioUnitario, Descuento: d.Descuento,
			PrecioFinal: d.PrecioFinal,
			// El total de la línea de la factura permite reconocer, al
			// calcular, el ICE que se dedujo de él (ver
			// comprobante.RecalcularImpuestos) aunque la factura se haya
			// leído de XML.
			PrecioTotalSinImpuesto: d.PrecioTotalSinImpuesto,
			DetallesAdicionales:    append(comprobante.Lista[comprobante.DetAdicional](nil), d.DetallesAdicionales...),
			Impuestos:              append(comprobante.Lista[comprobante.Impuesto](nil), d.Impuestos...),
		})
	}
	n.Calcular()
	n.AsociarFactura(f)
	return n
}

// LineaPrecioFinal construye un detalle definido por su precio final por
// unidad, con el IVA incluido; ver factura.LineaPrecioFinal.
func LineaPrecioFinal(codigo, descripcion string, cantidad, precioFinal decimal.Decimal, codigoIVA string) Detalle {
	d := LineaIVA(codigo, descripcion, cantidad, decimal.Cero, codigoIVA)
	d.PrecioFinal = precioFinal
	return d
}

// decimalesDePrecio devuelve los decimales que admiten la cantidad y el
// precio unitario en la versión del esquema.
func (n *NotaCredito) decimalesDePrecio() int {
	if n.version() == V100 {
		return 2
	}
	return 6
}

// impuestosFinales devuelve los impuestos de los detalles definidos por su
// precio final.
func (n *NotaCredito) impuestosFinales() [][]comprobante.Impuesto {
	var out [][]comprobante.Impuesto
	for _, d := range n.Detalles {
		if !d.PrecioFinal.EsCero() && comprobante.AdmitePrecioFinal(d.Impuestos) {
			out = append(out, d.Impuestos)
		}
	}
	return out
}

// DeclararPreciosFinales declara que la nota de crédito se emitió con
// precios finales (IVA incluido), y toma como precio final de cada detalle
// el que figura en él; ver factura.Factura.DeclararPreciosFinales.
func (n *NotaCredito) DeclararPreciosFinales() {
	for i := range n.Detalles {
		if pf, ok := comprobante.PrecioFinalDeclarado(n.Detalles[i].Cantidad, n.Detalles[i].Impuestos); ok {
			n.Detalles[i].PrecioFinal = pf
		}
	}
}

func (n *NotaCredito) impuestosPorLinea() [][]comprobante.Impuesto {
	out := make([][]comprobante.Impuesto, len(n.Detalles))
	for i, d := range n.Detalles {
		out[i] = d.Impuestos
	}
	return out
}

// Calcular completa los importes derivados de los detalles: total de cada
// línea, base y valor de sus impuestos, totalSinImpuestos,
// totalConImpuestos y valorModificacion. Mismos criterios que
// factura.Factura.Calcular, también al recalcular después de ajustar los
// detalles para una devolución parcial.
func (n *NotaCredito) Calcular() {
	total := decimal.Cero
	for i := range n.Detalles {
		d := &n.Detalles[i]
		d.Descuento = d.Descuento.Redondear(2)
		if pu, t, ok := comprobante.CalcularPrecioFinal(d.Cantidad, d.PrecioFinal, d.Descuento, n.decimalesDePrecio(), d.Impuestos); ok {
			d.PrecioUnitario, d.PrecioTotalSinImpuesto = pu, t
		} else {
			anterior := d.PrecioTotalSinImpuesto
			d.PrecioTotalSinImpuesto = comprobante.TotalLinea(d.Cantidad, d.PrecioUnitario, d.Descuento)
			comprobante.RecalcularImpuestos(anterior, d.PrecioTotalSinImpuesto, d.Impuestos)
		}
		total = total.Sumar(d.PrecioTotalSinImpuesto)
	}
	finales := n.impuestosFinales()
	info := &n.InfoNotaCredito
	info.TotalSinImpuestos = total.Redondear(2)
	info.TotalConImpuestos = nil
	valor := info.TotalSinImpuestos
	for _, g := range comprobante.AgruparImpuestos(n.impuestosPorLinea()...) {
		t := comprobante.TotalImpuesto{Codigo: g.Codigo, CodigoPorcentaje: g.CodigoPorcentaje, BaseImponible: g.Base.Redondear(2), Valor: g.ValorCon(finales)}
		// El XSD de la nota de crédito solo admite un total por
		// código/porcentaje sin tarifa: los grupos de igual código se suman.
		if k := len(info.TotalConImpuestos) - 1; k >= 0 && info.TotalConImpuestos[k].Codigo == t.Codigo && info.TotalConImpuestos[k].CodigoPorcentaje == t.CodigoPorcentaje {
			info.TotalConImpuestos[k].BaseImponible = info.TotalConImpuestos[k].BaseImponible.Sumar(t.BaseImponible)
			info.TotalConImpuestos[k].Valor = info.TotalConImpuestos[k].Valor.Sumar(t.Valor)
		} else {
			info.TotalConImpuestos = append(info.TotalConImpuestos, t)
		}
		valor = valor.Sumar(t.Valor)
	}
	for _, c := range info.Compensaciones {
		valor = valor.Restar(c.Valor)
	}
	info.ValorModificacion = valor.Redondear(2)
	if info.Moneda == "" {
		info.Moneda = comprobante.MonedaDolar
	}
}

// GenerarClaveAcceso compone la clave de acceso y la guarda en
// InfoTributaria.ClaveAcceso. Ver factura.Factura.GenerarClaveAcceso.
func (n *NotaCredito) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&n.InfoTributaria, n.InfoNotaCredito.FechaEmision, comprobante.CodNotaCredito, codigoNumerico)
}

var (
	reNumDoc     = regexp.MustCompile(`^[0-9]{3}-[0-9]{3}-[0-9]{9}$`)
	reContribEsp = regexp.MustCompile(`^[A-Za-z0-9]{3,13}$`)
)

// OpcionesValidacion ajusta ValidarCon.
type OpcionesValidacion struct {
	// Hoy es la fecha de referencia; por defecto, la actual en Ecuador.
	Hoy comprobante.Fecha
}

// Validar aplica las reglas que se pueden comprobar sin consultar al SRI y
// devuelve todos los hallazgos.
func (n *NotaCredito) Validar() comprobante.Hallazgos { return n.ValidarCon(OpcionesValidacion{}) }

// ValidarCon es Validar con opciones.
func (n *NotaCredito) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	ver := n.version()
	if ver != V100 && ver != V110 {
		v.Errorf("version", "version", "versión %q desconocida; existen %s y %s", ver, V100, V110)
	}
	v.InfoTributaria(n.InfoTributaria, comprobante.CodNotaCredito)
	info := &n.InfoNotaCredito
	const p = "infoNotaCredito."

	if info.FechaEmision.EsCero() {
		v.Errorf(p+"fechaEmision", "requerido", "es obligatoria")
	} else if info.FechaEmision.Time().After(hoy.Time()) {
		v.Errorf(p+"fechaEmision", "fecha.futura", "%s es posterior a la fecha actual en Ecuador (%s)", info.FechaEmision, hoy)
	}
	v.ClaveAcceso(n.InfoTributaria, info.FechaEmision)

	v.Texto(p+"dirEstablecimiento", info.DirEstablecimiento, false, 300)
	v.Identificacion(p+"tipoIdentificacionComprador", p+"identificacionComprador",
		info.TipoIdentificacionComprador, info.IdentificacionComprador, false)
	v.Texto(p+"razonSocialComprador", info.RazonSocialComprador, true, 300)
	if info.ContribuyenteEspecial != "" && !reContribEsp.MatchString(info.ContribuyenteEspecial) {
		v.Errorf(p+"contribuyenteEspecial", "formato", "debe tener de 3 a 13 caracteres alfanuméricos")
	}
	switch info.ObligadoContabilidad {
	case "", "SI", "NO":
	default:
		v.Errorf(p+"obligadoContabilidad", "catalogo", `debe ser "SI" o "NO"`)
	}
	v.Texto(p+"rise", info.Rise, false, 40)
	v.Texto(p+"moneda", info.Moneda, false, 15)
	v.Texto(p+"motivo", info.Motivo, true, 300)

	// --- Documento modificado -----------------------------------------------
	if len(info.CodDocModificado) != 2 || !comprobante.SoloDigitos(info.CodDocModificado) {
		v.Errorf(p+"codDocModificado", "formato", "debe ser un código de 2 dígitos (01 para factura)")
	}
	if !reNumDoc.MatchString(info.NumDocModificado) {
		v.Errorf(p+"numDocModificado", "formato", "debe tener el formato 001-001-000000001")
	}
	switch {
	case info.FechaEmisionDocSustento.EsCero():
		v.Errorf(p+"fechaEmisionDocSustento", "requerido", "es obligatoria")
	case !info.FechaEmision.EsCero() && info.FechaEmisionDocSustento.Time().After(info.FechaEmision.Time()):
		v.Errorf(p+"fechaEmisionDocSustento", "fecha.sustento", "el comprobante modificado (%s) no puede ser posterior a la nota de crédito (%s)",
			info.FechaEmisionDocSustento, info.FechaEmision)
	}

	// --- Detalles -----------------------------------------------------------
	decimales := 6
	if ver == V100 {
		decimales = 2
	}
	if len(n.Detalles) == 0 {
		v.Errorf("detalles", "requerido", "la nota de crédito debe tener al menos un detalle")
	}
	suma := decimal.Cero
	for i, d := range n.Detalles {
		q := fmt.Sprintf("detalles[%d].", i)
		v.Texto(q+"descripcion", d.Descripcion, true, 300)
		v.Texto(q+"codigoInterno", d.CodigoInterno, false, 25)
		v.Texto(q+"codigoAdicional", d.CodigoAdicional, false, 25)
		v.Numero(q+"cantidad", d.Cantidad, 18, decimales)
		v.Numero(q+"precioUnitario", d.PrecioUnitario, 18, decimales)
		v.Numero(q+"descuento", d.Descuento, 14, 2)
		v.Numero(q+"precioTotalSinImpuesto", d.PrecioTotalSinImpuesto, 14, 2)
		if esperado := comprobante.TotalLinea(d.Cantidad, d.PrecioUnitario, d.Descuento); !d.PrecioTotalSinImpuesto.Igual(esperado) {
			v.Errorf(q+"precioTotalSinImpuesto", "detalle.total", "es %s pero cantidad × precioUnitario − descuento = %s", d.PrecioTotalSinImpuesto, esperado)
		}
		if len(d.DetallesAdicionales) > 3 {
			v.Errorf(q+"detallesAdicionales", "maximo", "se admiten hasta 3 detalles adicionales, hay %d", len(d.DetallesAdicionales))
		}
		for j, da := range d.DetallesAdicionales {
			v.Texto(fmt.Sprintf("%sdetallesAdicionales[%d].nombre", q, j), da.Nombre, true, 300)
			v.Texto(fmt.Sprintf("%sdetallesAdicionales[%d].valor", q, j), da.Valor, true, 300)
		}
		if len(d.Impuestos) == 0 {
			v.Errorf(q+"impuestos", "requerido", "cada detalle debe declarar al menos un impuesto")
		}
		if d.PrecioFinal.EsCero() {
			v.ImpuestosDetalle(q, d.PrecioTotalSinImpuesto, d.Impuestos)
		} else {
			v.ImpuestosDetallePrecioFinal(q, d.PrecioTotalSinImpuesto, d.Cantidad, d.PrecioFinal, d.Impuestos)
		}
		suma = suma.Sumar(d.PrecioTotalSinImpuesto)
	}

	// --- Totales ------------------------------------------------------------
	v.Numero(p+"totalSinImpuestos", info.TotalSinImpuestos, 14, 2)
	v.Numero(p+"valorModificacion", info.ValorModificacion, 14, 2)
	if len(n.Detalles) > 0 && !info.TotalSinImpuestos.Igual(suma) {
		v.Errorf(p+"totalSinImpuestos", "totales.sin-impuestos", "es %s pero los detalles suman %s", info.TotalSinImpuestos, suma.Redondear(2))
	}
	for i, t := range info.TotalConImpuestos {
		if t.Tarifa != nil || t.DescuentoAdicional != nil {
			v.Errorf(fmt.Sprintf("%stotalConImpuestos[%d]", p, i), "esquema", "en la nota de crédito un totalImpuesto no lleva tarifa ni descuentoAdicional")
		}
	}
	impuestos := v.TotalesImpuestos(p+"totalConImpuestos", info.TotalConImpuestos, n.impuestosPorLinea()...)
	v.TotalesPrecioFinal(p+"totalConImpuestos", info.TotalConImpuestos, n.impuestosFinales(), n.impuestosPorLinea()...)
	v.Compensaciones(p+"compensaciones", info.Compensaciones, info.TotalConImpuestos)
	esperado := info.TotalSinImpuestos.Sumar(impuestos)
	for _, c := range info.Compensaciones {
		esperado = esperado.Restar(c.Valor)
	}
	if esperado = esperado.Redondear(2); !info.ValorModificacion.Igual(esperado) {
		v.Errorf(p+"valorModificacion", "totales.importe", "es %s pero totalSinImpuestos + impuestos = %s", info.ValorModificacion, esperado)
	}

	// --- Información adicional ---------------------------------------------
	if len(n.InfoAdicional) > 15 {
		v.Errorf("infoAdicional", "maximo", "se admiten hasta 15 campos adicionales, hay %d (sección 9.11)", len(n.InfoAdicional))
	}
	for i, c := range n.InfoAdicional {
		v.Texto(fmt.Sprintf("infoAdicional[%d].nombre", i), c.Nombre, true, 300)
		v.Texto(fmt.Sprintf("infoAdicional[%d].valor", i), c.Valor, true, 300)
		if c.Nombre == comprobante.NombreRUCProveedor {
			if err := comprobante.ValidarRUC(c.Valor); err != nil {
				v.Errorf(fmt.Sprintf("infoAdicional[%d].valor", i), "ruc.formato", "el campo %q debe contener el RUC del proveedor (Anexo 26): %v", c.Nombre, err)
			}
		}
	}
	if m := n.MaquinaFiscal; m != nil {
		v.Texto("maquinaFiscal.marca", m.Marca, true, 30)
		v.Texto("maquinaFiscal.modelo", m.Modelo, true, 30)
		v.Texto("maquinaFiscal.serie", m.Serie, true, 30)
	}
	if n.origen != nil {
		n.validarContra(v, n.origen.factura, n.origen.anteriores)
	}
	return v.Hallazgos()
}

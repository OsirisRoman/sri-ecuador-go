// Package notadebito modela la nota de débito electrónica del SRI
// (codDoc 05), versión 1.0.0: estructura tipada, serialización y lectura de
// XML, cálculo de totales y validación.
//
// Una nota de débito aumenta el valor de un comprobante de venta ya emitido
// (intereses de mora, gastos, recargos). No tiene detalles de productos:
// lleva una lista de motivos, cada uno con su razón y su valor, y los
// impuestos se declaran sobre el total.
//
// Referencia: ficha técnica v2.34, Anexo 1 (formato XML nota de débito). La
// tarifa de IVA es la vigente a la fecha de emisión del documento de
// sustento.
package notadebito

import (
	"encoding/xml"
	"fmt"
	"regexp"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Version es la única versión del esquema de nota de débito.
const Version = "1.0.0"

// NotaDebito es el comprobante <notaDebito>.
type NotaDebito struct {
	XMLName xml.Name `xml:"notaDebito"`
	ID      string   `xml:"id,attr"`
	Version string   `xml:"version,attr"`

	InfoTributaria comprobante.InfoTributaria                    `xml:"infoTributaria"`
	InfoNotaDebito InfoNotaDebito                                `xml:"infoNotaDebito"`
	Motivos        comprobante.Lista[Motivo]                     `xml:"motivos"`
	MaquinaFiscal  *comprobante.MaquinaFiscal                    `xml:"maquinaFiscal,omitempty"`
	InfoAdicional  comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`
}

// InfoNotaDebito es el bloque <infoNotaDebito>.
type InfoNotaDebito struct {
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
	// Comprobante que se modifica.
	CodDocModificado        string            `xml:"codDocModificado"`
	NumDocModificado        string            `xml:"numDocModificado"`
	FechaEmisionDocSustento comprobante.Fecha `xml:"fechaEmisionDocSustento"`

	// TotalSinImpuestos es la suma de los valores de los motivos.
	TotalSinImpuestos decimal.Decimal `xml:"totalSinImpuestos"`
	// Impuestos sobre el total. Con un único impuesto, Calcular toma como
	// base el total sin impuestos; con varios, indique la base de cada uno.
	Impuestos      comprobante.Lista[Impuesto]                 `xml:"impuestos"`
	Compensaciones comprobante.Lista[comprobante.Compensacion] `xml:"compensaciones,omitempty"`
	ValorTotal     decimal.Decimal                             `xml:"valorTotal"`
	Pagos          comprobante.Lista[comprobante.Pago]         `xml:"pagos,omitempty"`
}

// Impuesto es un <impuesto> de la nota de débito: como el de un detalle de
// factura, más el valor de devolución de IVA (Anexo 20).
type Impuesto struct {
	Codigo           string          `xml:"codigo"`
	CodigoPorcentaje string          `xml:"codigoPorcentaje"`
	Tarifa           decimal.Decimal `xml:"tarifa"`
	BaseImponible    decimal.Decimal `xml:"baseImponible"`
	Valor            decimal.Decimal `xml:"valor"`
	// ValorDevolucionIva solo puede omitirse o ser cero: Validar rechaza un
	// valor positivo porque el SDK no implementa la devolución automática
	// del IVA (ver comprobante.Validador.DevolucionIVA). Es el monto de devolución automática de IVA
	// autorizado por el servicio DIG del SRI (Anexo 20). El SRI lo contrasta
	// y rechaza con el error 92 si no coincide.
	ValorDevolucionIva *decimal.Decimal `xml:"valorDevolucionIva,omitempty"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Impuesto) EtiquetaXML() string { return "impuesto" }

func (i Impuesto) comun() comprobante.Impuesto {
	return comprobante.Impuesto{Codigo: i.Codigo, CodigoPorcentaje: i.CodigoPorcentaje, Tarifa: i.Tarifa, BaseImponible: i.BaseImponible, Valor: i.Valor}
}

// Motivo es un <motivo> de la nota de débito.
type Motivo struct {
	Razon string          `xml:"razon"`
	Valor decimal.Decimal `xml:"valor"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Motivo) EtiquetaXML() string { return "motivo" }

// Info devuelve el bloque infoTributaria.
func (n *NotaDebito) Info() comprobante.InfoTributaria { return n.InfoTributaria }

// XML serializa la nota de débito como documento XML UTF-8 listo para
// firmar.
func (n *NotaDebito) XML() ([]byte, error) {
	copia := *n
	copia.XMLName = xml.Name{Local: "notaDebito"}
	copia.ID = "comprobante"
	if copia.Version == "" {
		copia.Version = Version
	}
	return comprobante.Serializar(&copia)
}

// Parse lee una nota de débito desde su XML, firmado o no.
func Parse(doc []byte) (*NotaDebito, error) {
	var n NotaDebito
	if err := xml.Unmarshal(doc, &n); err != nil {
		return nil, fmt.Errorf("notadebito: XML ilegible: %w", err)
	}
	return &n, nil
}

// ImpuestoIVA devuelve el impuesto IVA con el código de la Tabla 17
// indicado, listo para InfoNotaDebito.Impuestos; Calcular completa tarifa,
// base y valor.
func ImpuestoIVA(codigoIVA string) Impuesto {
	return Impuesto{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: codigoIVA}
}

// Calcular completa totalSinImpuestos (suma de los motivos), la tarifa,
// base y valor de los impuestos, y valorTotal. Si hay un único pago sin
// total se le asigna el valor total.
func (n *NotaDebito) Calcular() {
	total := decimal.Cero
	for i := range n.Motivos {
		n.Motivos[i].Valor = n.Motivos[i].Valor.Redondear(2)
		total = total.Sumar(n.Motivos[i].Valor)
	}
	info := &n.InfoNotaDebito
	info.TotalSinImpuestos = total.Redondear(2)
	valor := info.TotalSinImpuestos
	for i := range info.Impuestos {
		imp := &info.Impuestos[i]
		if len(info.Impuestos) == 1 || imp.BaseImponible.EsCero() {
			imp.BaseImponible = info.TotalSinImpuestos
		}
		imp.BaseImponible = imp.BaseImponible.Redondear(2)
		if imp.Codigo == comprobante.ImpuestoIVA {
			if t, ok := comprobante.TarifasIVA[imp.CodigoPorcentaje]; ok && t.Fija {
				imp.Tarifa = t.Tarifa
			}
			imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa)
		} else if imp.Valor.EsCero() && !imp.Tarifa.EsCero() {
			imp.Valor = imp.BaseImponible.Porcentaje(imp.Tarifa)
		}
		imp.Valor = imp.Valor.Redondear(2)
		valor = valor.Sumar(imp.Valor)
	}
	for _, c := range info.Compensaciones {
		valor = valor.Restar(c.Valor)
	}
	info.ValorTotal = valor.Redondear(2)
	if len(info.Pagos) == 1 && info.Pagos[0].Total.EsCero() {
		info.Pagos[0].Total = info.ValorTotal
	}
}

// GenerarClaveAcceso compone la clave de acceso y la guarda en
// InfoTributaria.ClaveAcceso. Ver factura.Factura.GenerarClaveAcceso.
func (n *NotaDebito) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&n.InfoTributaria, n.InfoNotaDebito.FechaEmision, comprobante.CodNotaDebito, codigoNumerico)
}

var (
	reNumDoc       = regexp.MustCompile(`^[0-9]{3}-[0-9]{3}-[0-9]{9}$`)
	reContribEsp   = regexp.MustCompile(`^[A-Za-z0-9]{3,13}$`)
	reFormaPagoXSD = regexp.MustCompile(`^(0[1-9]|1[0-9]|2[01])$`)
)

// OpcionesValidacion ajusta ValidarCon.
type OpcionesValidacion struct {
	// Hoy es la fecha de referencia; por defecto, la actual en Ecuador.
	Hoy comprobante.Fecha
}

// Validar aplica las reglas que se pueden comprobar sin consultar al SRI y
// devuelve todos los hallazgos.
func (n *NotaDebito) Validar() comprobante.Hallazgos { return n.ValidarCon(OpcionesValidacion{}) }

// ValidarCon es Validar con opciones.
func (n *NotaDebito) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	if n.Version != "" && n.Version != Version {
		v.Errorf("version", "version", "versión %q desconocida; solo existe %s", n.Version, Version)
	}
	v.InfoTributaria(n.InfoTributaria, comprobante.CodNotaDebito)
	info := &n.InfoNotaDebito
	const p = "infoNotaDebito."

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
		v.Errorf(p+"fechaEmisionDocSustento", "fecha.sustento", "el comprobante modificado (%s) no puede ser posterior a la nota de débito (%s)",
			info.FechaEmisionDocSustento, info.FechaEmision)
	}

	// --- Motivos e importes ---------------------------------------------------
	if len(n.Motivos) == 0 {
		v.Errorf("motivos", "requerido", "la nota de débito debe tener al menos un motivo")
	}
	suma := decimal.Cero
	for i, m := range n.Motivos {
		v.Texto(fmt.Sprintf("motivos[%d].razon", i), m.Razon, true, 300)
		v.Numero(fmt.Sprintf("motivos[%d].valor", i), m.Valor, 14, 2)
		suma = suma.Sumar(m.Valor)
	}
	v.Numero(p+"totalSinImpuestos", info.TotalSinImpuestos, 14, 2)
	v.Numero(p+"valorTotal", info.ValorTotal, 14, 2)
	if len(n.Motivos) > 0 && !info.TotalSinImpuestos.Igual(suma) {
		v.Errorf(p+"totalSinImpuestos", "totales.sin-impuestos", "es %s pero los motivos suman %s", info.TotalSinImpuestos, suma.Redondear(2))
	}
	if len(info.Impuestos) == 0 {
		v.Errorf(p+"impuestos", "requerido", "debe declarar al menos un impuesto (use IVA 0 %% si no grava)")
	}
	esperado, bases := info.TotalSinImpuestos, decimal.Cero
	for i, imp := range info.Impuestos {
		v.ImpuestoLinea(fmt.Sprintf("%simpuestos[%d].", p, i), imp.comun())
		v.DevolucionIVA(fmt.Sprintf("%simpuestos[%d].valorDevolucionIva", p, i), imp.ValorDevolucionIva)
		esperado = esperado.Sumar(imp.Valor)
		if imp.Codigo == comprobante.ImpuestoIVA {
			bases = bases.Sumar(imp.BaseImponible)
		}
	}
	if bases.Cmp(info.TotalSinImpuestos) > 0 {
		v.Errorf(p+"impuestos", "totales.base", "las bases de IVA suman %s, más que el total sin impuestos (%s)", bases.Redondear(2), info.TotalSinImpuestos)
	}
	for _, c := range info.Compensaciones {
		esperado = esperado.Restar(c.Valor)
	}
	if esperado = esperado.Redondear(2); !info.ValorTotal.Igual(esperado) {
		v.Errorf(p+"valorTotal", "totales.importe", "es %s pero totalSinImpuestos + impuestos = %s", info.ValorTotal, esperado)
	}

	// --- Pagos ------------------------------------------------------------------
	sumaPagos := decimal.Cero
	for i, pg := range info.Pagos {
		q := fmt.Sprintf("%spagos[%d].", p, i)
		switch _, enTabla := comprobante.FormasPago[pg.FormaPago]; {
		case !reFormaPagoXSD.MatchString(pg.FormaPago):
			v.Errorf(q+"formaPago", "catalogo", "código %q inválido (Tabla 24)", pg.FormaPago)
		case !enTabla:
			v.Advertirf(q+"formaPago", "catalogo", "el código %q no figura entre las formas de pago vigentes de la Tabla 24", pg.FormaPago)
		}
		v.Numero(q+"total", pg.Total, 14, 2)
		v.Texto(q+"unidadTiempo", pg.UnidadTiempo, false, 10)
		sumaPagos = sumaPagos.Sumar(pg.Total)
	}
	if len(info.Pagos) > 0 && !sumaPagos.Igual(info.ValorTotal) {
		v.Advertirf(p+"pagos", "pagos.suma", "los pagos suman %s y el valor total es %s", sumaPagos.Redondear(2), info.ValorTotal)
	}

	// --- Información adicional -------------------------------------------------
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
	return v.Hallazgos()
}

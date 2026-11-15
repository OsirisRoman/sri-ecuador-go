package factura

import (
	"fmt"
	"regexp"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// LimiteConsumidorFinal es el importe máximo de una factura a consumidor
// final: por encima deben consignarse los datos del adquirente (ficha
// técnica v2.34, sección 9.10).
var LimiteConsumidorFinal = decimal.Entero(50)

var (
	reGuia          = regexp.MustCompile(`^[0-9]{3}-[0-9]{3}-[0-9]{9}$`)
	reContribEsp    = regexp.MustCompile(`^[A-Za-z0-9]{3,13}$`)
	reFormaPagoXSD  = regexp.MustCompile(`^(0[1-9]|1[0-9]|2[01])$`)
	maxInfoAdic     = 15
	maxDetAdicional = 3
)

// OpcionesValidacion ajusta Validar.
type OpcionesValidacion struct {
	// Hoy es la fecha contra la que se compara la de emisión; por defecto,
	// la fecha actual en Ecuador.
	Hoy comprobante.Fecha
}

// Validar aplica las reglas de la ficha técnica que se pueden comprobar sin
// consultar al SRI y devuelve todos los hallazgos, no solo el primero.
//
// Complementa a la validación XSD (paquete esquema), que comprueba la forma;
// aquí se comprueba el contenido: coherencia de la clave de acceso con el
// comprobante (rechazo 58 del SRI), aritmética de detalles y totales
// (rechazo 52), catálogos, identificaciones y requisitos de llenado.
//
// Use Hallazgos.Err para obtener un error solo si hay incumplimientos que
// impedirían la autorización.
func (f *Factura) Validar() comprobante.Hallazgos { return f.ValidarCon(OpcionesValidacion{}) }

// ValidarCon es Validar con opciones.
func (f *Factura) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	ver := f.version()
	switch ver {
	case V100, V110, V200, V210:
	default:
		v.Errorf("version", "version", "versión %q desconocida; existen %s, %s, %s y %s", ver, V100, V110, V200, V210)
	}
	esV2 := ver == V200 || ver == V210
	if ver == V100 && len(f.Retenciones) > 0 {
		v.Errorf("retenciones", "version", "el bloque retenciones no existe en la versión 1.0.0")
	}
	if !esV2 {
		if f.InfoSustitutivaGuiaRemision != nil {
			v.Errorf("infoSustitutivaGuiaRemision", "version", "solo existe en las versiones 2.0.0 y 2.1.0 (Anexo 9)")
		}
		if len(f.OtrosRubrosTerceros) > 0 {
			v.Errorf("otrosRubrosTerceros", "version", "solo existe en las versiones 2.0.0 y 2.1.0 (Anexo 8)")
		}
	}

	v.InfoTributaria(f.InfoTributaria, comprobante.CodFactura)
	info := &f.InfoFactura

	// --- Fecha y clave de acceso -------------------------------------------
	if info.FechaEmision.EsCero() {
		v.Errorf("infoFactura.fechaEmision", "requerido", "es obligatoria")
	} else if info.FechaEmision.Time().After(hoy.Time()) {
		v.Errorf("infoFactura.fechaEmision", "fecha.futura", "%s es posterior a la fecha actual en Ecuador (%s)", info.FechaEmision, hoy)
	}
	v.ClaveAcceso(f.InfoTributaria, info.FechaEmision)

	// --- Emisor --------------------------------------------------------------
	v.Texto("infoFactura.dirEstablecimiento", info.DirEstablecimiento, false, 300)
	if info.ContribuyenteEspecial != "" && !reContribEsp.MatchString(info.ContribuyenteEspecial) {
		v.Errorf("infoFactura.contribuyenteEspecial", "formato", "debe tener de 3 a 13 caracteres alfanuméricos")
	}
	switch info.ObligadoContabilidad {
	case "", "SI", "NO":
	default:
		v.Errorf("infoFactura.obligadoContabilidad", "catalogo", `debe ser "SI" o "NO"`)
	}

	// --- Comprador -----------------------------------------------------------
	v.Identificacion("infoFactura.tipoIdentificacionComprador", "infoFactura.identificacionComprador",
		info.TipoIdentificacionComprador, info.IdentificacionComprador, true)
	v.Texto("infoFactura.razonSocialComprador", info.RazonSocialComprador, true, 300)
	v.Texto("infoFactura.direccionComprador", info.DireccionComprador, false, 300)
	if info.TipoIdentificacionComprador == comprobante.IDConsumidorFinal && info.ImporteTotal.Cmp(LimiteConsumidorFinal) > 0 {
		v.Errorf("infoFactura.tipoIdentificacionComprador", "consumidor-final.monto",
			"una factura a consumidor final no puede superar USD %s; identifique al adquirente (sección 9.10)", LimiteConsumidorFinal)
	}
	if info.GuiaRemision != "" && !reGuia.MatchString(info.GuiaRemision) {
		v.Errorf("infoFactura.guiaRemision", "formato", "debe tener el formato 001-001-000000001")
	}
	v.Texto("infoFactura.moneda", info.Moneda, false, 15)
	v.Texto("infoFactura.placa", info.Placa, false, 20)

	// --- Detalles ------------------------------------------------------------
	decimalesCantidad := 6
	if ver == V100 || ver == V200 {
		decimalesCantidad = 2
	}
	if len(f.Detalles) == 0 {
		v.Errorf("detalles", "requerido", "la factura debe tener al menos un detalle")
	}
	sumaLineas, sumaDesc := decimal.Cero, decimal.Cero
	for i, d := range f.Detalles {
		p := fmt.Sprintf("detalles[%d].", i)
		v.Texto(p+"descripcion", d.Descripcion, true, 300)
		v.Texto(p+"codigoPrincipal", d.CodigoPrincipal, false, 25)
		v.Texto(p+"codigoAuxiliar", d.CodigoAuxiliar, false, 25)
		if d.CodigoPrincipal == "" {
			v.Advertirf(p+"codigoPrincipal", "requerido", "la ficha técnica lo marca como obligatorio aunque el XSD lo admite vacío")
		}
		v.Numero(p+"cantidad", d.Cantidad, 18, decimalesCantidad)
		v.Numero(p+"precioUnitario", d.PrecioUnitario, 18, decimalesCantidad)
		v.Numero(p+"descuento", d.Descuento, 14, 2)
		v.Numero(p+"precioTotalSinImpuesto", d.PrecioTotalSinImpuesto, 14, 2)
		if esperado := totalLinea(d); !d.PrecioTotalSinImpuesto.Igual(esperado) {
			v.Errorf(p+"precioTotalSinImpuesto", "detalle.total",
				"es %s pero cantidad × precioUnitario − descuento = %s", d.PrecioTotalSinImpuesto, esperado)
		}
		if len(d.DetallesAdicionales) > maxDetAdicional {
			v.Errorf(p+"detallesAdicionales", "maximo", "se admiten hasta %d detalles adicionales, hay %d", maxDetAdicional, len(d.DetallesAdicionales))
		}
		for j, da := range d.DetallesAdicionales {
			v.Texto(fmt.Sprintf("%sdetallesAdicionales[%d].nombre", p, j), da.Nombre, true, 300)
			v.Texto(fmt.Sprintf("%sdetallesAdicionales[%d].valor", p, j), da.Valor, true, 300)
		}
		if len(d.Impuestos) == 0 {
			v.Errorf(p+"impuestos", "requerido", "cada detalle debe declarar al menos un impuesto (use IVA 0%%, no objeto o exento si no grava)")
		}
		for j, imp := range d.Impuestos {
			v.ImpuestoLinea(fmt.Sprintf("%simpuestos[%d].", p, j), imp)
		}
		sumaLineas = sumaLineas.Sumar(d.PrecioTotalSinImpuesto)
		sumaDesc = sumaDesc.Sumar(d.Descuento)
	}

	// --- Totales -------------------------------------------------------------
	v.Numero("infoFactura.totalSinImpuestos", info.TotalSinImpuestos, 14, 2)
	v.Numero("infoFactura.totalDescuento", info.TotalDescuento, 14, 2)
	v.Numero("infoFactura.propina", info.Propina, 14, 2)
	v.Numero("infoFactura.importeTotal", info.ImporteTotal, 14, 2)
	if len(f.Detalles) > 0 {
		if !info.TotalSinImpuestos.Igual(sumaLineas) {
			v.Errorf("infoFactura.totalSinImpuestos", "totales.sin-impuestos", "es %s pero los detalles suman %s", info.TotalSinImpuestos, sumaLineas.Redondear(2))
		}
		if !info.TotalDescuento.Igual(sumaDesc) {
			v.Errorf("infoFactura.totalDescuento", "totales.descuento", "es %s pero los descuentos de los detalles suman %s", info.TotalDescuento, sumaDesc.Redondear(2))
		}
	}
	impuestos := v.TotalesImpuestos("infoFactura.totalConImpuestos", info.TotalConImpuestos, f.impuestosPorLinea()...)
	if esperado := f.importeEsperado(impuestos); !info.ImporteTotal.Igual(esperado) {
		v.Errorf("infoFactura.importeTotal", "totales.importe",
			"es %s pero totalSinImpuestos + impuestos + propina y demás rubros = %s", info.ImporteTotal, esperado)
	}

	// --- Pagos ---------------------------------------------------------------
	if len(info.Pagos) == 0 {
		v.Errorf("infoFactura.pagos", "requerido", "debe indicar al menos una forma de pago (Tabla 24)")
	}
	sumaPagos := decimal.Cero
	for i, pg := range info.Pagos {
		p := fmt.Sprintf("infoFactura.pagos[%d].", i)
		switch _, enTabla := comprobante.FormasPago[pg.FormaPago]; {
		case !reFormaPagoXSD.MatchString(pg.FormaPago):
			v.Errorf(p+"formaPago", "catalogo", "código %q inválido (Tabla 24)", pg.FormaPago)
		case !enTabla:
			v.Advertirf(p+"formaPago", "catalogo", "el código %q no figura entre las formas de pago vigentes de la Tabla 24", pg.FormaPago)
		}
		v.Numero(p+"total", pg.Total, 14, 2)
		v.Texto(p+"unidadTiempo", pg.UnidadTiempo, false, 10)
		sumaPagos = sumaPagos.Sumar(pg.Total)
	}
	if len(info.Pagos) > 0 && !sumaPagos.Igual(info.ImporteTotal) {
		v.Advertirf("infoFactura.pagos", "pagos.suma", "los pagos suman %s y el importe total es %s", sumaPagos.Redondear(2), info.ImporteTotal)
	}

	// --- Información adicional ----------------------------------------------
	if len(f.InfoAdicional) > maxInfoAdic {
		v.Errorf("infoAdicional", "maximo", "se admiten hasta %d campos adicionales, hay %d (sección 9.11)", maxInfoAdic, len(f.InfoAdicional))
	}
	for i, c := range f.InfoAdicional {
		v.Texto(fmt.Sprintf("infoAdicional[%d].nombre", i), c.Nombre, true, 300)
		v.Texto(fmt.Sprintf("infoAdicional[%d].valor", i), c.Valor, true, 300)
		if c.Nombre == comprobante.NombreRUCProveedor {
			if err := comprobante.ValidarRUC(c.Valor); err != nil {
				v.Errorf(fmt.Sprintf("infoAdicional[%d].valor", i), "ruc.formato", "el campo %q debe contener el RUC del proveedor (Anexo 26): %v", c.Nombre, err)
			}
		}
	}
	if f.TipoNegociable != nil {
		v.Texto("tipoNegociable.correo", f.TipoNegociable.Correo, true, 100)
	}
	if m := f.MaquinaFiscal; m != nil {
		v.Texto("maquinaFiscal.marca", m.Marca, true, 30)
		v.Texto("maquinaFiscal.modelo", m.Modelo, true, 30)
		v.Texto("maquinaFiscal.serie", m.Serie, true, 30)
	}
	for i, r := range f.OtrosRubrosTerceros {
		v.Texto(fmt.Sprintf("otrosRubrosTerceros[%d].concepto", i), r.Concepto, true, 300)
		v.Numero(fmt.Sprintf("otrosRubrosTerceros[%d].total", i), r.Total, 14, 2)
	}
	return v.Hallazgos()
}

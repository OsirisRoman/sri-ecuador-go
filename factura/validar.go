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

var reGuia = regexp.MustCompile(`^[0-9]{3}-[0-9]{3}-[0-9]{9}$`)

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
	v.Emisor("infoFactura.", info.DirEstablecimiento, info.ContribuyenteEspecial, info.ObligadoContabilidad)

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
	sumaLineas, sumaDesc := ValidarDetalles(v, f.Detalles, decimalesCantidad)
	VigenciaDetalles(v, f.Detalles, info.FechaEmision)

	// --- Totales -------------------------------------------------------------
	v.Numero("infoFactura.totalSinImpuestos", info.TotalSinImpuestos, 14, 2)
	v.Numero("infoFactura.totalDescuento", info.TotalDescuento, 14, 2)
	v.Numero("infoFactura.propina", info.Propina, 14, 2)
	if limite := info.TotalSinImpuestos.Porcentaje(decimal.Entero(10)); info.Propina.Cmp(limite) > 0 {
		// El facturador del SRI no admite una propina mayor que el 10 % del
		// subtotal (sección 9.16, campo 9). No es una regla del esquema ni de
		// la tabla de errores, por eso es una advertencia.
		v.Advertirf("infoFactura.propina", "propina.limite", "es %s, más del 10 %% del subtotal (%s); el facturador del SRI no lo admite (sección 9.16)", info.Propina, limite.Redondear(2))
	}
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

	// --- Pagos e información adicional ------------------------------------
	v.Pagos("infoFactura.pagos", info.Pagos, info.ImporteTotal, true)
	v.Adicionales(f.InfoAdicional, f.MaquinaFiscal)
	// --- Exportación, reembolsos y requisitos por actividad ---------------
	f.validarExportacion(v)
	ValidarReembolsos(v, "infoFactura.", DatosReembolso{info.CodDocReembolso, info.TotalComprobantesReembolso,
		info.TotalBaseImponibleReembolso, info.TotalImpuestoReembolso}, f.Reembolsos, true)
	f.validarActividades(v)
	for i, r := range f.OtrosRubrosTerceros {
		v.Texto(fmt.Sprintf("otrosRubrosTerceros[%d].concepto", i), r.Concepto, true, 300)
		v.Numero(fmt.Sprintf("otrosRubrosTerceros[%d].total", i), r.Total, 14, 2)
	}
	return v.Hallazgos()
}

// VigenciaDetalles advierte de los IVA de tarifa general que no estaban
// vigentes en la fecha de emisión (comprobante.Validador.VigenciaIVA). Lo
// comparten la factura y la liquidación de compra.
func VigenciaDetalles(v *comprobante.Validador, detalles []Detalle, emision comprobante.Fecha) {
	for i, d := range detalles {
		for j, imp := range d.Impuestos {
			v.VigenciaIVA(fmt.Sprintf("detalles[%d].impuestos[%d].codigoPorcentaje", i, j), imp, emision)
		}
	}
}

// ValidarDetalles aplica a v las reglas de los detalles de un comprobante
// de venta (forma, aritmética de cada línea e impuestos) y devuelve la suma
// de los totales de línea y la de los descuentos. decimales es el máximo de
// decimales de cantidad y precio unitario que admite la versión del esquema.
//
// Lo comparten la factura y la liquidación de compra, cuyos detalles son
// idénticos.
func ValidarDetalles(v *comprobante.Validador, detalles []Detalle, decimales int) (sumaLineas, sumaDescuentos decimal.Decimal) {
	for i, d := range detalles {
		p := fmt.Sprintf("detalles[%d].", i)
		v.Texto(p+"descripcion", d.Descripcion, true, 300)
		v.Texto(p+"codigoPrincipal", d.CodigoPrincipal, false, 25)
		v.Texto(p+"codigoAuxiliar", d.CodigoAuxiliar, false, 25)
		v.Texto(p+"unidadMedida", d.UnidadMedida, false, 50)
		if d.CodigoPrincipal == "" {
			v.Advertirf(p+"codigoPrincipal", "requerido", "la ficha técnica lo marca como obligatorio aunque el XSD lo admite vacío")
		}
		v.Numero(p+"cantidad", d.Cantidad, 18, decimales)
		v.Numero(p+"precioUnitario", d.PrecioUnitario, 18, decimales)
		v.Numero(p+"descuento", d.Descuento, 14, 2)
		v.Numero(p+"precioTotalSinImpuesto", d.PrecioTotalSinImpuesto, 14, 2)
		if esperado := totalLinea(d); !d.PrecioTotalSinImpuesto.Igual(esperado) {
			v.Errorf(p+"precioTotalSinImpuesto", "detalle.total",
				"es %s pero cantidad × precioUnitario − descuento = %s", d.PrecioTotalSinImpuesto, esperado)
		}
		v.DetallesAdicionales(p, d.DetallesAdicionales)
		if len(d.Impuestos) == 0 {
			v.Errorf(p+"impuestos", "requerido", "cada detalle debe declarar al menos un impuesto (use IVA 0%%, no objeto o exento si no grava)")
		}
		v.ImpuestosDetalle(p, d.PrecioTotalSinImpuesto, d.Impuestos)
		sumaLineas = sumaLineas.Sumar(d.PrecioTotalSinImpuesto)
		sumaDescuentos = sumaDescuentos.Sumar(d.Descuento)
	}
	return sumaLineas, sumaDescuentos
}

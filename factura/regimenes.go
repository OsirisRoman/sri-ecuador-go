package factura

import (
	"fmt"
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// ComercioExteriorExportador es el único valor de <comercioExterior>
// (Anexo 4).
const ComercioExteriorExportador = "EXPORTADOR"

// CodDocReembolso es el valor de <codDocReembolso> en un comprobante que
// incluye reembolsos de gastos (Anexo 5).
const CodDocReembolso = "41"

// Códigos auxiliares de los servicios de transporte comercial, excepto
// taxis (Anexo 25, Tabla 32).
const (
	// CodAuxTransporteOperadora va en las facturas de la operadora a sus
	// clientes; exige además la placa del vehículo.
	CodAuxTransporteOperadora = "H492001"
	// CodAuxTransporteSocio va en las facturas del socio o accionista a la
	// operadora.
	CodAuxTransporteSocio = "H492002"
)

// Combustibles es la Tabla 30 (Anexo 16): código principal y descripción
// que deben llevar los detalles de las facturas de venta de combustibles.
var Combustibles = map[string]string{
	"0103": "SÚPER",
	"0101": "EXTRA",
	"0174": "EXTRA CON ETANOL",
	"0121": "DIESEL PREMIUM",
	"0104": "DIESEL 2",
}

// TotalesReembolso suma las bases imponibles y los impuestos de los
// comprobantes reembolsados.
func TotalesReembolso(rs []ReembolsoDetalle) (base, impuesto decimal.Decimal) {
	for _, r := range rs {
		for _, di := range r.DetalleImpuestos {
			base = base.Sumar(di.BaseImponibleReembolso)
			impuesto = impuesto.Sumar(di.ImpuestoReembolso)
		}
	}
	return base.Redondear(2), impuesto.Redondear(2)
}

// DatosReembolso son los totales de reembolso que declara el bloque de
// información de una factura o de una liquidación de compra.
type DatosReembolso struct {
	CodDocReembolso                                                                 string
	TotalComprobantesReembolso, TotalBaseImponibleReembolso, TotalImpuestoReembolso *decimal.Decimal
}

// ValidarReembolsos comprueba los reembolsos de gastos (Anexo 5): que
// codDocReembolso sea 41 cuando los hay, la forma de cada comprobante
// reembolsado y que los totales declarados coincidan con su suma. p es la
// ruta del bloque de información terminada en punto; compensaciones indica
// si el esquema admite <compensacionesReembolso>.
//
// Lo comparten la factura y la liquidación de compra.
func ValidarReembolsos(v *comprobante.Validador, p string, d DatosReembolso, rs []ReembolsoDetalle, compensaciones bool) {
	if len(rs) == 0 {
		if d.CodDocReembolso == CodDocReembolso {
			v.Errorf("reembolsos", "requerido", "codDocReembolso es %s pero no hay reembolsos", CodDocReembolso)
		}
		return
	}
	if d.CodDocReembolso != CodDocReembolso {
		v.Errorf(p+"codDocReembolso", "reembolso.codigo", "con reembolsos debe ser %s; use Calcular", CodDocReembolso)
	}
	for i, r := range rs {
		q := fmt.Sprintf("reembolsos[%d].", i)
		v.Texto(q+"identificacionProveedorReembolso", r.IdentificacionProveedorReembolso, true, 20)
		v.Pais(q+"codPaisPagoProveedorReembolso", r.CodPaisPagoProveedorReembolso)
		v.Digitos(q+"estabDocReembolso", r.EstabDocReembolso, 3)
		v.Digitos(q+"ptoEmiDocReembolso", r.PtoEmiDocReembolso, 3)
		v.Digitos(q+"secuencialDocReembolso", r.SecuencialDocReembolso, 9)
		if r.FechaEmisionDocReembolso.EsCero() {
			v.Errorf(q+"fechaEmisionDocReembolso", "requerido", "es obligatoria")
		}
		if n := len(r.NumeroAutorizacionDocReemb); !comprobante.SoloDigitos(r.NumeroAutorizacionDocReemb) || (n != 10 && n != 37 && n != 49) {
			v.Errorf(q+"numeroautorizacionDocReemb", "formato", "debe tener 10, 37 o 49 dígitos")
		}
		if len(r.DetalleImpuestos) == 0 {
			v.Errorf(q+"detalleImpuestos", "requerido", "cada reembolso debe declarar al menos un impuesto")
		}
		if !compensaciones && len(r.CompensacionesReembolso) > 0 {
			v.Errorf(q+"compensacionesReembolso", "version", "este comprobante no admite compensaciones en los reembolsos")
		}
		for j, di := range r.DetalleImpuestos {
			v.Numero(fmt.Sprintf("%sdetalleImpuestos[%d].baseImponibleReembolso", q, j), di.BaseImponibleReembolso, 14, 2)
			v.Numero(fmt.Sprintf("%sdetalleImpuestos[%d].impuestoReembolso", q, j), di.ImpuestoReembolso, 14, 2)
		}
	}
	base, imp := TotalesReembolso(rs)
	comparar := func(campo string, declarado *decimal.Decimal, esperado decimal.Decimal) {
		if declarado == nil || !declarado.Igual(esperado) {
			v.Errorf(p+campo, "reembolso.totales", "debe ser %s (suma de los reembolsos)", esperado)
		}
	}
	comparar("totalBaseImponibleReembolso", d.TotalBaseImponibleReembolso, base)
	comparar("totalImpuestoReembolso", d.TotalImpuestoReembolso, imp)
	comparar("totalComprobantesReembolso", d.TotalComprobantesReembolso, base.Sumar(imp))
}

// validarExportacion aplica el Anexo 4: los campos de exportación van
// juntos y solo en facturas de exportación.
func (f *Factura) validarExportacion(v *comprobante.Validador) {
	i := &f.InfoFactura
	const p = "infoFactura."
	propios := []struct{ campo, valor string }{
		{"incoTermFactura", i.IncoTermFactura}, {"lugarIncoTerm", i.LugarIncoTerm}, {"paisOrigen", i.PaisOrigen},
		{"puertoEmbarque", i.PuertoEmbarque}, {"puertoDestino", i.PuertoDestino}, {"paisDestino", i.PaisDestino},
		{"paisAdquisicion", i.PaisAdquisicion}, {"incoTermTotalSinImpuestos", i.IncoTermTotalSinImpuestos},
	}
	importes := []struct {
		campo string
		valor *decimal.Decimal
	}{
		{"fleteInternacional", i.FleteInternacional}, {"seguroInternacional", i.SeguroInternacional},
		{"gastosAduaneros", i.GastosAduaneros}, {"gastosTransporteOtros", i.GastosTransporteOtros},
	}
	for _, m := range importes {
		if m.valor != nil {
			v.Numero(p+m.campo, *m.valor, 14, 2)
		}
	}
	if i.ComercioExterior == "" {
		for _, c := range propios {
			if c.valor != "" {
				v.Errorf(p+c.campo, "exportacion.comercio-exterior", "es un campo de exportación: indique comercioExterior = %q (Anexo 4)", ComercioExteriorExportador)
			}
		}
		for _, m := range importes {
			if m.valor != nil {
				v.Errorf(p+m.campo, "exportacion.comercio-exterior", "es un campo de exportación: indique comercioExterior = %q (Anexo 4)", ComercioExteriorExportador)
			}
		}
		return
	}
	if i.ComercioExterior != ComercioExteriorExportador {
		v.Errorf(p+"comercioExterior", "catalogo", "debe ser %q", ComercioExteriorExportador)
	}
	v.Texto(p+"incoTermFactura", i.IncoTermFactura, true, 10)
	v.Texto(p+"lugarIncoTerm", i.LugarIncoTerm, true, 300)
	v.Texto(p+"puertoEmbarque", i.PuertoEmbarque, true, 300)
	v.Texto(p+"puertoDestino", i.PuertoDestino, true, 300)
	v.Texto(p+"incoTermTotalSinImpuestos", i.IncoTermTotalSinImpuestos, true, 10)
	for _, c := range propios {
		if strings.HasPrefix(c.campo, "pais") {
			v.Pais(p+c.campo, c.valor)
		}
	}
}

// placaSinFormato informa si la placa no sigue las Tablas 29 y 33: letras
// mayúsculas y dígitos, sin espacios ni guiones.
func placaSinFormato(placa string) bool {
	for _, r := range placa {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return true
		}
	}
	return false
}

// validarActividades aplica los requisitos de llenado que dependen de lo
// que se vende, cuando el propio comprobante permite reconocerlo:
// combustibles (Anexos 12 y 16), transporte comercial (Anexo 25) y factura
// comercial negociable (Anexo 11). Salvo la negociable, son advertencias:
// el SDK no puede saber con certeza la actividad del emisor.
func (f *Factura) validarActividades(v *comprobante.Validador) {
	i := &f.InfoFactura
	const p = "infoFactura."
	combustible, transporte := -1, -1
	for n, d := range f.Detalles {
		if nombre, ok := Combustibles[d.CodigoPrincipal]; ok && strings.EqualFold(strings.TrimSpace(d.Descripcion), nombre) && combustible < 0 {
			combustible = n
		}
		if d.CodigoAuxiliar == CodAuxTransporteOperadora && transporte < 0 {
			transporte = n
		}
	}
	if i.Placa == "" {
		if combustible >= 0 {
			v.Advertirf(p+"placa", "combustible.placa", "detalles[%d] es un combustible de la Tabla 30: la factura debe llevar la placa del vehículo (Anexo 12)", combustible)
		}
		if transporte >= 0 {
			v.Advertirf(p+"placa", "transporte.placa", "detalles[%d] lleva el código auxiliar %s: la factura de la operadora de transporte debe llevar la placa del vehículo (Anexo 25)", transporte, CodAuxTransporteOperadora)
		}
	} else if placaSinFormato(i.Placa) {
		v.Advertirf(p+"placa", "placa.formato", "%q: la placa se escribe con letras mayúsculas y números, sin espacios ni guiones, y con un cero antes de los números si solo son tres (ABC0123; Tablas 29 y 33)", i.Placa)
	}

	if f.TipoNegociable != nil {
		v.Texto("tipoNegociable.correo", f.TipoNegociable.Correo, true, 100)
		if c := f.TipoNegociable.Correo; c != "" && (!strings.Contains(c, "@") || strings.ContainsAny(c, " \t")) {
			v.Errorf("tipoNegociable.correo", "formato", "%q no es una dirección de correo", c)
		}
		if i.DireccionComprador == "" {
			v.Errorf(p+"direccionComprador", "negociable.direccion", "la factura comercial negociable exige la dirección del comprador (Anexo 11)")
		}
		conPlazo := false
		for _, pg := range i.Pagos {
			conPlazo = conPlazo || (pg.Plazo != nil && pg.UnidadTiempo != "")
		}
		if !conPlazo {
			v.Errorf(p+"pagos", "negociable.plazo", "la factura comercial negociable exige una forma de pago con plazo y unidad de tiempo (Anexo 11)")
		}
	}
}

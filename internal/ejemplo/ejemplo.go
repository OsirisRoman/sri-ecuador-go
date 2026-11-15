// Package ejemplo construye comprobantes ficticios para pruebas y ejemplos.
// Ningún dato corresponde a un contribuyente real.
package ejemplo

import (
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// RUC es el RUC ficticio del emisor de ejemplo.
const RUC = "1790000000001"

// Factura devuelve una factura válida en ambiente de pruebas, ya calculada
// y con clave de acceso, emitida en la fecha indicada.
func Factura(fecha comprobante.Fecha, secuencial string) *factura.Factura {
	f := &factura.Factura{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, TipoEmision: comprobante.EmisionNormal,
			RazonSocial: "EMISOR DE PRUEBA S.A.", RUC: RUC, CodDoc: comprobante.CodFactura,
			Estab: "001", PtoEmi: "001", Secuencial: secuencial,
			DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoFactura: factura.InfoFactura{
			FechaEmision:                fecha,
			ObligadoContabilidad:        "NO",
			TipoIdentificacionComprador: comprobante.IDRuc,
			RazonSocialComprador:        "PRUEBAS SERVICIO DE RENTAS INTERNAS",
			IdentificacionComprador:     "1760013210001",
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}},
		},
		Detalles: []factura.Detalle{
			factura.LineaIVA("SRV-001", "Servicio de prueba", decimal.Entero(1), decimal.MustParse("10.00"), comprobante.IVA15),
		},
	}
	f.Calcular()
	if _, err := f.GenerarClaveAcceso("12345678"); err != nil {
		panic(err)
	}
	return f
}

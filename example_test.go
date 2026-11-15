package sri_test

import (
	"context"
	"fmt"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

// Flujo completo de una factura contra el simulador incluido en el SDK. En
// una aplicación real el certificado se carga con certificado.CargarArchivo
// y no se sustituyen los Servicios.
func Example() {
	material, _ := certprueba.Obtener() // certificado desechable de pruebas
	simulador := sritest.Nuevo()
	defer simulador.Cerrar()

	cliente, err := sri.Nuevo(sri.Config{
		Ambiente:    comprobante.Pruebas,
		Certificado: material.Certificado,
		Servicios:   ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(simulador.Endpoints())),
		Espera:      sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 3},
	})
	if err != nil {
		panic(err)
	}

	f := &factura.Factura{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente:    comprobante.Pruebas,
			RazonSocial: "EMISOR DE PRUEBA S.A.",
			RUC:         "1790000000001",
			Estab:       "001", PtoEmi: "001", Secuencial: "000000001",
			DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoFactura: factura.InfoFactura{
			FechaEmision:                comprobante.Hoy(),
			TipoIdentificacionComprador: comprobante.IDConsumidorFinal,
			RazonSocialComprador:        comprobante.RazonSocialConsumidorFinal,
			IdentificacionComprador:     comprobante.IdentificacionConsumidorFinal,
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}},
		},
		Detalles: []factura.Detalle{
			factura.LineaIVA("CAF-01", "Café americano", decimal.Entero(2), decimal.MustParse("1.75"), comprobante.IVA15),
		},
	}
	f.Calcular()                                                // totales e impuestos
	if _, err := f.GenerarClaveAcceso("12345678"); err != nil { // clave de 49 dígitos
		panic(err)
	}

	res, err := cliente.Emitir(context.Background(), f)
	if err != nil {
		panic(err)
	}
	fmt.Println("importe total:", f.InfoFactura.ImporteTotal)
	fmt.Println("estado:", res.Estado)
	for _, m := range res.Mensajes {
		fmt.Println(m.Tipo, m)
	}
	// Output:
	// importe total: 4.03
	// estado: AUTORIZADO
	// ADVERTENCIA [60] ESTE PROCESO FUE REALIZADO EN EL AMBIENTE DE PRUEBAS
}

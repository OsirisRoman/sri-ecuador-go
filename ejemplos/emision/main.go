// Comando emision es el ejemplo de referencia de la guía de integración
// (docs/guia-de-integracion.md): emite una factura de principio a fin y
// muestra qué guardar y qué hacer en cada estado.
//
// Sin configuración se ejecuta contra el simulador incluido (sritest) con un
// certificado desechable, así que no envía nada al SRI:
//
//	go run ./ejemplos/emision
//
// Con SRI_P12_PATH y SRI_P12_PASSWORD definidos usa ese certificado contra
// el ambiente de PRUEBAS del SRI (nunca producción) y consume el secuencial
// indicado en SRI_SECUENCIAL.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/ride"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

func main() {
	if err := ejecutar(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func ejecutar() error {
	cliente, ruc, cerrar, err := nuevoCliente()
	if err != nil {
		return err
	}
	defer cerrar()

	// 1. Construir el comprobante. El secuencial lo asigna SU sistema:
	// consecutivo por establecimiento, punto de emisión y tipo de
	// comprobante, sin huecos ni repeticiones.
	secuencial, _ := strconv.Atoi(os.Getenv("SRI_SECUENCIAL"))
	if secuencial < 1 {
		secuencial = 1
	}
	f := &factura.Factura{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente:    comprobante.Pruebas,
			RazonSocial: valor("SRI_EMISOR_RAZON_SOCIAL", "EMISOR DE PRUEBA S.A."),
			RUC:         ruc,
			Estab:       "001", PtoEmi: "001",
			Secuencial: fmt.Sprintf("%09d", secuencial),
			DirMatriz:  valor("SRI_EMISOR_DIR_MATRIZ", "Av. Amazonas N34-10 y Roca"),
		},
		InfoFactura: factura.InfoFactura{
			FechaEmision:                comprobante.Hoy(), // fecha de Ecuador, no la del servidor
			ObligadoContabilidad:        "NO",
			TipoIdentificacionComprador: comprobante.IDCedula,
			RazonSocialComprador:        "PRUEBAS SERVICIO DE RENTAS INTERNAS",
			IdentificacionComprador:     "1710034065",
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}},
		},
		Detalles: []factura.Detalle{
			// Importes siempre como decimal.Decimal, nunca float64.
			factura.LineaIVA("SRV-01", "Servicio de ejemplo", decimal.Entero(2), decimal.MustParse("12.50"), comprobante.IVA15),
			factura.LineaIVA("LIB-02", "Libro (tarifa 0 %)", decimal.Entero(1), decimal.MustParse("8.00"), comprobante.IVA0),
		},
	}

	// 2. Calcular totales y generar la clave de acceso UNA sola vez.
	f.Calcular()
	claveAcceso, err := f.GenerarClaveAcceso("")
	if err != nil {
		return err
	}

	// 3. Preparar: valida reglas y XSD y firma. No se comunica con el SRI.
	prep, err := cliente.Preparar(f)
	var ev *sri.ErrorValidacion
	if errors.As(err, &ev) {
		for _, h := range ev.Hallazgos {
			fmt.Println("  corregir:", h)
		}
		return errors.New("el comprobante no cumple las reglas; no se firmó ni se envió")
	}
	if err != nil {
		return err
	}

	// 4. GUARDAR antes de enviar: clave de acceso y XML firmado. Si el
	// proceso se cae a mitad del envío, con esto se reanuda sin duplicar.
	guardar(claveAcceso, "FIRMADO", prep.XMLFirmado)

	// 5. Enviar y esperar la autorización. Reanudar es idempotente: puede
	// repetirse con el mismo XML firmado las veces que haga falta.
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelar()
	res, err := cliente.Reanudar(ctx, prep.XMLFirmado, prep.Advertencias)
	if err != nil {
		// Fallo de comunicación: el estado es DESCONOCIDO. No genere otra
		// clave ni otro secuencial; reintente Reanudar más tarde.
		return fmt.Errorf("sin respuesta del SRI, reintentar con el mismo XML: %w", err)
	}

	// 6. Actuar según el estado.
	switch res.Estado {
	case sri.EstadoAutorizado:
		autorizado := res.XMLAutorizado() // el documento con validez tributaria
		guardar(claveAcceso, "AUTORIZADO", autorizado)
		pdf, err := ride.Generar(autorizado, nil) // &ride.Opciones{Logo: …} para el logotipo
		if err != nil {
			return err
		}
		fmt.Printf("AUTORIZADO: factura %s, importe %s, RIDE de %d bytes\n",
			f.InfoTributaria.NumeroComprobante(), f.InfoFactura.ImporteTotal, len(pdf))
		// Aquí se entrega al cliente el XML autorizado y el RIDE (paquete
		// correo u otro medio).
	case sri.EstadoRecibido:
		// El SRI lo aceptó pero aún no resuelve. No es un comprobante
		// válido todavía: consultar más tarde con cliente.Autorizar(clave).
		guardar(claveAcceso, "RECIBIDO", prep.XMLFirmado)
		fmt.Println("RECIBIDO: pendiente de autorización; consultar más tarde")
	case sri.EstadoDevuelto, sri.EstadoNoAutorizado:
		// Rechazado. Corregir la causa y volver a emitir: el secuencial
		// puede reutilizarse.
		guardar(claveAcceso, res.Estado.String(), prep.XMLFirmado)
		for _, m := range res.Mensajes {
			fmt.Println("  SRI:", m.Tipo, m)
		}
		return fmt.Errorf("comprobante %s", res.Estado)
	}
	return nil
}

// guardar representa la persistencia de su aplicación (base de datos,
// almacenamiento de objetos…).
func guardar(claveAcceso, estado string, documento []byte) {
	fmt.Printf("guardar: clave …%s estado %s (%d bytes)\n", claveAcceso[len(claveAcceso)-8:], estado, len(documento))
}

func valor(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}

// nuevoCliente devuelve un Cliente del ambiente de pruebas. Con un .p12
// configurado habla con el SRI; sin él, con el simulador incluido.
func nuevoCliente() (cliente *sri.Cliente, ruc string, cerrar func(), err error) {
	if ruta := os.Getenv("SRI_P12_PATH"); ruta != "" {
		// La contraseña llega por el entorno o un gestor de secretos; nunca
		// va en el código ni en el repositorio.
		cert, err := certificado.CargarArchivo(ruta, os.Getenv("SRI_P12_PASSWORD"))
		if err != nil {
			return nil, "", nil, err
		}
		ruc = os.Getenv("SRI_EMISOR_RUC")
		// Config.RUC hace que el cliente se niegue a firmar para otro RUC.
		cliente, err = sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: ruc})
		return cliente, ruc, func() {}, err
	}
	material, err := certprueba.Obtener() // solo para este ejemplo y para pruebas
	if err != nil {
		return nil, "", nil, err
	}
	simulador := sritest.Nuevo()
	cliente, err = sri.Nuevo(sri.Config{
		Ambiente:    comprobante.Pruebas,
		Certificado: material.Certificado,
		Servicios:   ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(simulador.Endpoints())),
		Espera:      sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 5},
	})
	return cliente, "1790000000001", simulador.Cerrar, err
}

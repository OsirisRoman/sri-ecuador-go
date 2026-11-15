// Comando qa ejecuta escenarios de interoperabilidad contra el ambiente de
// PRUEBAS del SRI con el certificado y el emisor configurados en .env.
//
// Es una herramienta de mantenimiento del SDK: consume secuenciales reales
// del ambiente de pruebas del emisor. Nunca opera contra producción.
//
//	go run ./internal/qa -desde 113            # todos los escenarios
//	go run ./internal/qa -desde 120 -solo sha1 # uno solo
//
// Los XML y un resumen se guardan en -dir (por defecto evidencia-privada/,
// excluido de Git). La salida estándar no incluye datos del emisor.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/entorno"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

// Comprador de pruebas que sugiere la ficha técnica (sección 5.6, Tabla 5).
const (
	razonPruebas = "PRUEBAS SERVICIO DE RENTAS INTERNAS"
	rucSRI       = "1760013210001"
)

type escenario struct {
	nombre, descripcion string
	algoritmo           xades.Algoritmo
	construir           func(f *factura.Factura)
	// alterar, si no es nil, modifica el documento firmado antes del primer
	// envío; después se envía el original con la misma clave (ficha 5.10).
	alterar func(firmado string) string
}

func consumidorFinal(f *factura.Factura) {
	f.InfoFactura.TipoIdentificacionComprador = comprobante.IDConsumidorFinal
	f.InfoFactura.RazonSocialComprador = comprobante.RazonSocialConsumidorFinal
	f.InfoFactura.IdentificacionComprador = comprobante.IdentificacionConsumidorFinal
	f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}}
	f.Detalles = []factura.Detalle{factura.LineaIVA("P-001", "PRODUCTO DE PRUEBA", d("1"), d("1.00"), comprobante.IVA15)}
}

func compradorRUC(f *factura.Factura) {
	f.InfoFactura.TipoIdentificacionComprador = comprobante.IDRuc
	f.InfoFactura.RazonSocialComprador = razonPruebas
	f.InfoFactura.IdentificacionComprador = rucSRI
	f.InfoFactura.DireccionComprador = "Av. Amazonas y Unión Nacional de Periodistas"
}

var escenarios = []escenario{
	{nombre: "completa-1.1.0", descripcion: "v1.1.0, comprador RUC, IVA 15 % con descuento y 6 decimales + IVA 0 %, crédito 30 días, info adicional con ñ & <>",
		construir: func(f *factura.Factura) {
			compradorRUC(f)
			l1 := factura.LineaIVA("SRV-001", "Servicio de diseño & desarrollo <web>", d("2.5"), d("19.123456"), comprobante.IVA15)
			l1.Descuento = d("3.21")
			l1.CodigoAuxiliar = "AUX-9"
			l1.DetallesAdicionales = []comprobante.DetAdicional{{Nombre: "Período", Valor: "Noviembre 2026"}}
			l2 := factura.LineaIVA("LIB-002", "Libro técnico (tarifa 0 %)", d("3"), d("12.50"), comprobante.IVA0)
			f.Detalles = []factura.Detalle{l1, l2}
			plazo := d("30")
			f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero, Plazo: &plazo, UnidadTiempo: "dias"}}
			f.InfoAdicional = []comprobante.CampoAdicional{
				{Nombre: "Email", Valor: "pruebas@example.com"},
				{Nombre: "Observación", Valor: `Ñandú & Cía. "comillas" <ángulos> 'apóstrofo'`},
			}
		}},
	{nombre: "cedula-2.1.0", descripcion: "v2.1.0, comprador con cédula (la del propio emisor), IVA 15 % + IVA 5 %, dos formas de pago, rubro de terceros",
		construir: func(f *factura.Factura) {
			f.Version = factura.V210
			f.InfoFactura.TipoIdentificacionComprador = comprobante.IDCedula
			f.InfoFactura.RazonSocialComprador = razonPruebas
			f.InfoFactura.IdentificacionComprador = f.InfoTributaria.RUC[:10]
			f.Detalles = []factura.Detalle{
				factura.LineaIVA("A-1", "Artículo gravado 15 %", d("4"), d("2.375"), comprobante.IVA15),
				factura.LineaIVA("M-2", "Material tarifa 5 %", d("10"), d("1.10"), comprobante.IVA5),
			}
			f.OtrosRubrosTerceros = []factura.Rubro{{Concepto: "Tasa de terceros", Total: d("0.50")}}
			f.InfoFactura.Pagos = []comprobante.Pago{
				{FormaPago: comprobante.PagoSinSistemaFinanciero, Total: d("5.00")},
				{FormaPago: comprobante.PagoTarjetaCredito},
			}
		}},
	{nombre: "minima-1.0.0", descripcion: "v1.0.0, consumidor final",
		construir: func(f *factura.Factura) { f.Version = factura.V100; consumidorFinal(f) }},
	{nombre: "minima-2.0.0", descripcion: "v2.0.0, consumidor final",
		construir: func(f *factura.Factura) { f.Version = factura.V200; consumidorFinal(f) }},
	{nombre: "exento-no-objeto", descripcion: "v1.1.0, líneas exenta (7) y no objeto de IVA (6), propina",
		construir: func(f *factura.Factura) {
			compradorRUC(f)
			f.Detalles = []factura.Detalle{
				factura.LineaIVA("E-1", "Bien exento de IVA", d("1"), d("8.00"), comprobante.IVAExento),
				factura.LineaIVA("N-1", "Concepto no objeto de IVA", d("1"), d("4.00"), comprobante.IVANoObjeto),
				factura.LineaIVA("G-1", "Consumo gravado", d("1"), d("10.00"), comprobante.IVA15),
			}
			f.InfoFactura.Propina = d("1.00")
			f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoTarjetaDebito}}
		}},
	{nombre: "sha1", descripcion: "v1.1.0 mínima firmada con RSA-SHA1 (algoritmo de la ficha 6.8)", algoritmo: xades.SHA1,
		construir: consumidorFinal},
	{nombre: "firma-alterada", descripcion: "contenido modificado tras firmar; luego el original con la misma clave (5.10)",
		construir: consumidorFinal,
		alterar: func(s string) string {
			return strings.Replace(s, "PRODUCTO DE PRUEBA", "PRODUCTO ALTERADO", 1)
		}},
	{nombre: "xsd-invalido", descripcion: "elemento no permitido por el XSD; luego el original con la misma clave",
		construir: consumidorFinal,
		alterar: func(s string) string {
			return strings.Replace(s, "<infoFactura>", "<noExiste>1</noExiste><infoFactura>", 1)
		}},
}

func pagosPendientes(f *factura.Factura) {
	// El último pago sin total absorbe el saldo.
	f.Calcular()
	ps := f.InfoFactura.Pagos
	if len(ps) < 2 {
		return
	}
	resto := f.InfoFactura.ImporteTotal
	for i := 0; i < len(ps)-1; i++ {
		resto = resto.Restar(ps[i].Total)
	}
	ps[len(ps)-1].Total = resto.Redondear(2)
}

func main() {
	desde := flag.Int("desde", 0, "primer secuencial a usar (obligatorio)")
	solo := flag.String("solo", "", "ejecuta solo los escenarios indicados, separados por comas")
	ndDesde := flag.Int("nd-desde", 0, "secuencial de nota de débito; activa el escenario de nota de débito")
	ncDesde := flag.Int("nc-desde", 0, "primer secuencial de nota de crédito; activa los escenarios de nota de crédito")
	dir := flag.String("dir", "evidencia-privada", "directorio de evidencias (excluido de Git)")
	flag.Parse()
	if *desde < 1 {
		fmt.Fprintln(os.Stderr, "indique -desde con el primer secuencial libre")
		os.Exit(2)
	}
	entorno.CargarEnv(".env")
	cert, err := entorno.Certificado()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	ronda := filepath.Join(*dir, "qa-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(ronda, 0o700); err != nil {
		panic(err)
	}
	resumen, _ := os.OpenFile(filepath.Join(ronda, "resumen.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	defer resumen.Close()
	decir := func(formato string, args ...any) {
		linea := fmt.Sprintf(formato, args...)
		fmt.Println(linea)
		fmt.Fprintln(resumen, linea)
	}

	sec := *desde
	fallos := 0
	for _, e := range escenarios {
		if *solo != "" && !strings.Contains(","+*solo+",", ","+e.nombre+",") {
			continue
		}
		if *solo == "" && (*ncDesde > 0 || *ndDesde > 0) {
			continue // con -nc-desde o -nd-desde y sin -solo no se repiten las facturas
		}
		decir("\n== %s (secuencial %d, %s)\n   %s", e.nombre, sec, e.algoritmo, e.descripcion)
		emisor, err := entorno.Emisor(comprobante.CodFactura, sec)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		f := &factura.Factura{InfoTributaria: emisor}
		f.InfoFactura.FechaEmision = comprobante.Hoy()
		f.InfoFactura.DirEstablecimiento = entorno.Valor("SRI_EMISOR_DIR_ESTABLECIMIENTO", emisor.DirMatriz)
		f.InfoFactura.ObligadoContabilidad = entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO")
		e.construir(f)
		pagosPendientes(f)
		if _, err := f.GenerarClaveAcceso(""); err != nil {
			decir("   ERROR local: %v", err)
			fallos++
			continue
		}
		cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, Algoritmo: e.algoritmo, RUC: emisor.RUC})
		if err != nil {
			panic(err)
		}
		prep, err := cli.Preparar(f)
		if err != nil {
			decir("   ERROR local (no se envió): %v", err)
			fallos++
			continue
		}
		for _, h := range prep.Advertencias {
			decir("   advertencia local: %s", h)
		}
		decir("   local: reglas OK, XSD OK, firma verificada; importe %s; versión %s", f.InfoFactura.ImporteTotal, valor(f.Version, factura.VersionPorDefecto))
		base := filepath.Join(ronda, fmt.Sprintf("%d-%s", sec, e.nombre))
		os.WriteFile(base+".firmado.xml", prep.XMLFirmado, 0o600)
		sec++ // el secuencial queda consumido desde que se envía

		if e.alterar != nil {
			alterado := e.alterar(string(prep.XMLFirmado))
			os.WriteFile(base+".alterado.xml", []byte(alterado), 0o600)
			decir("   -- envío del documento alterado")
			enviar(cli, []byte(alterado), base+".alterado", decir)
			decir("   -- envío del documento original (misma clave de acceso)")
		}
		if !enviar(cli, prep.XMLFirmado, base, decir) {
			fallos++
		}
	}
	secNC := *ncDesde
	if *ncDesde > 0 {
		for _, parcial := range []bool{false, true} {
			nombre := "nota-credito-total"
			if parcial {
				nombre = "nota-credito-parcial"
			}
			if *solo != "" && !strings.Contains(","+*solo+",", ","+nombre+",") {
				continue
			}
			decir("\n== %s (factura %d, nota de crédito %d)", nombre, sec, secNC)
			emisor, err := entorno.Emisor(comprobante.CodFactura, sec)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			f := &factura.Factura{InfoTributaria: emisor}
			f.InfoFactura.FechaEmision = comprobante.Hoy()
			f.InfoFactura.DirEstablecimiento = entorno.Valor("SRI_EMISOR_DIR_ESTABLECIMIENTO", emisor.DirMatriz)
			f.InfoFactura.ObligadoContabilidad = entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO")
			compradorRUC(f)
			f.Detalles = []factura.Detalle{
				factura.LineaIVA("A-1", "Artículo gravado", d("4"), d("12.345678"), comprobante.IVA15),
				factura.LineaIVA("B-2", "Artículo tarifa 0 %", d("2"), d("5.00"), comprobante.IVA0),
			}
			f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
			f.Calcular()
			f.GenerarClaveAcceso("")
			cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: emisor.RUC})
			if err != nil {
				panic(err)
			}
			prep, err := cli.Preparar(f)
			if err != nil {
				decir("   ERROR local en la factura: %v", err)
				fallos++
				continue
			}
			sec++
			decir("   -- factura de sustento, importe %s", f.InfoFactura.ImporteTotal)
			if !enviar(cli, prep.XMLFirmado, filepath.Join(ronda, fmt.Sprintf("%d-factura-para-nc", sec-1)), decir) {
				fallos++
				continue
			}
			n := notacredito.DesdeFactura(f, "Devolución de mercadería (prueba)")
			if parcial {
				n.Detalles = n.Detalles[:1]
				n.Detalles[0].Cantidad = d("1.5")
				n.InfoNotaCredito.Motivo = "Devolución parcial (prueba)"
				n.Calcular()
			}
			n.InfoTributaria.Secuencial = fmt.Sprintf("%09d", secNC)
			n.GenerarClaveAcceso("")
			prepNC, err := cli.Preparar(n)
			if err != nil {
				decir("   ERROR local en la nota de crédito: %v", err)
				fallos++
				continue
			}
			secNC++
			base := filepath.Join(ronda, fmt.Sprintf("nc-%d-%s", secNC-1, nombre))
			os.WriteFile(base+".firmado.xml", prepNC.XMLFirmado, 0o600)
			decir("   -- nota de crédito v%s, valor %s", valor(n.Version, notacredito.VersionPorDefecto), n.InfoNotaCredito.ValorModificacion)
			if !enviar(cli, prepNC.XMLFirmado, base, decir) {
				fallos++
			}
		}
	}
	if *ndDesde > 0 {
		decir("\n== nota-debito (factura %d, nota de débito %d)", sec, *ndDesde)
		emisor, err := entorno.Emisor(comprobante.CodFactura, sec)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		f := &factura.Factura{InfoTributaria: emisor}
		f.InfoFactura.FechaEmision = comprobante.Hoy()
		f.InfoFactura.ObligadoContabilidad = entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO")
		compradorRUC(f)
		f.Detalles = []factura.Detalle{factura.LineaIVA("A-1", "Servicio a crédito", d("1"), d("100.00"), comprobante.IVA15)}
		f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
		f.Calcular()
		f.GenerarClaveAcceso("")
		cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: emisor.RUC})
		if err != nil {
			panic(err)
		}
		if prep, err := cli.Preparar(f); err != nil {
			decir("   ERROR local en la factura: %v", err)
			fallos++
		} else {
			sec++
			decir("   -- factura de sustento, importe %s", f.InfoFactura.ImporteTotal)
			if enviar(cli, prep.XMLFirmado, filepath.Join(ronda, fmt.Sprintf("%d-factura-para-nd", sec-1)), decir) {
				itND, _ := entorno.Emisor(comprobante.CodNotaDebito, *ndDesde)
				n := &notadebito.NotaDebito{
					InfoTributaria: itND,
					InfoNotaDebito: notadebito.InfoNotaDebito{
						FechaEmision:                comprobante.Hoy(),
						DirEstablecimiento:          f.InfoFactura.DirEstablecimiento,
						ObligadoContabilidad:        f.InfoFactura.ObligadoContabilidad,
						TipoIdentificacionComprador: f.InfoFactura.TipoIdentificacionComprador,
						RazonSocialComprador:        f.InfoFactura.RazonSocialComprador,
						IdentificacionComprador:     f.InfoFactura.IdentificacionComprador,
						CodDocModificado:            comprobante.CodFactura,
						NumDocModificado:            f.InfoTributaria.NumeroComprobante(),
						FechaEmisionDocSustento:     f.InfoFactura.FechaEmision,
						Impuestos:                   []notadebito.Impuesto{notadebito.ImpuestoIVA(comprobante.IVA15)},
						Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}},
					},
					Motivos: []notadebito.Motivo{{Razon: "Interés por mora (prueba)", Valor: d("3.45")}, {Razon: "Gastos de cobranza (prueba)", Valor: d("1.50")}},
				}
				n.Calcular()
				n.GenerarClaveAcceso("")
				if prepND, err := cli.Preparar(n); err != nil {
					decir("   ERROR local en la nota de débito: %v", err)
					fallos++
				} else {
					base := filepath.Join(ronda, fmt.Sprintf("nd-%d", *ndDesde))
					os.WriteFile(base+".firmado.xml", prepND.XMLFirmado, 0o600)
					decir("   -- nota de débito v%s, valor total %s", notadebito.Version, n.InfoNotaDebito.ValorTotal)
					if !enviar(cli, prepND.XMLFirmado, base, decir) {
						fallos++
					}
				}
			} else {
				fallos++
			}
		}
	}
	decir("\nsecuenciales de factura usados: %d a %d; de nota de crédito: %d; escenarios no autorizados: %d", *desde, sec-1, secNC-*ncDesde, fallos)
	decir("evidencias: %s", ronda)
}

func valor(v, porDefecto string) string {
	if v == "" {
		return porDefecto
	}
	return v
}

// enviar entrega el documento y describe el resultado sin datos del emisor.
func enviar(cli *sri.Cliente, doc []byte, base string, decir func(string, ...any)) bool {
	ctx, cancelar := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancelar()
	res, err := cli.Reanudar(ctx, doc, nil)
	if res != nil {
		if rec := res.Recepcion; rec != nil {
			decir("   recepción: %s", rec.Estado)
			for _, m := range rec.Mensajes() {
				decir("     %s %s", m.Tipo, sanear(m.String()))
			}
		}
		decir("   resultado: %s", res.Estado)
		if res.Autorizacion != nil {
			decir("   estado SRI: %q", res.Autorizacion.Estado)
			for _, m := range res.Autorizacion.Mensajes {
				decir("     %s %s", m.Tipo, sanear(m.String()))
			}
		}
		if x := res.XMLAutorizado(); x != nil {
			os.WriteFile(base+".autorizado.xml", x, 0o600)
			decir("   comprobante devuelto idéntico al enviado: %t", res.Autorizacion.Comprobante == string(doc))
		}
	}
	if err != nil {
		decir("   ERROR de comunicación: %v", err)
		return false
	}
	return res.Autorizado()
}

// sanear oculta secuencias largas de dígitos (claves de acceso, RUC).
func sanear(s string) string {
	var sb strings.Builder
	n := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && s[i] >= '0' && s[i] <= '9' {
			n++
			continue
		}
		tramo := s[i-n : i]
		if n >= 10 {
			tramo = fmt.Sprintf("<%d dígitos>", n)
		}
		sb.WriteString(tramo)
		n = 0
		if i < len(s) {
			sb.WriteByte(s[i])
		}
	}
	if len(sb.String()) > 400 {
		return sb.String()[:400] + "…"
	}
	return sb.String()
}

// Comando qa ejecuta escenarios de interoperabilidad contra el ambiente de
// PRUEBAS del SRI con el certificado y el emisor configurados en .env.
//
// Es una herramienta de mantenimiento del SDK: consume secuenciales reales
// del ambiente de pruebas del emisor. Nunca opera contra producción.
//
//	go run ./internal/qa -desde 113            # todos los escenarios
//	go run ./internal/qa -desde 120 -solo sha1 # uno solo
//	go run ./internal/qa -regresion            # sin conexión: revalida lo ya autorizado
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
	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/guiaremision"
	"github.com/OsirisRoman/sri-ecuador-go/internal/entorno"
	"github.com/OsirisRoman/sri-ecuador-go/liquidacion"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
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

// Escenarios de regímenes específicos de factura; se ejecutan solo con
// -solo.
func init() {
	escenarios = append(escenarios,
		escenario{nombre: "exportacion", descripcion: "v1.1.0 de exportación (Anexo 4): comprador del exterior, IVA 0 %, incoterms, flete y seguro",
			construir: func(f *factura.Factura) {
				i := &f.InfoFactura
				i.TipoIdentificacionComprador = comprobante.IDExterior
				i.RazonSocialComprador = razonPruebas
				i.IdentificacionComprador = "EXT-123456"
				i.DireccionComprador = "Shanghai, China"
				i.ComercioExterior = factura.ComercioExteriorExportador
				i.IncoTermFactura, i.LugarIncoTerm = "CIF", "GUAYAQUIL"
				i.PaisOrigen, i.PaisDestino, i.PaisAdquisicion = "593", "331", "331"
				i.PuertoEmbarque, i.PuertoDestino = "GUAYAQUIL", "SHANGHAI"
				i.IncoTermTotalSinImpuestos = "FOB"
				flete, seguro := d("100.00"), d("20.50")
				i.FleteInternacional, i.SeguroInternacional = &flete, &seguro
				f.Detalles = []factura.Detalle{factura.LineaIVA("CACAO", "Cacao en grano para exportación", d("10"), d("120.00"), comprobante.IVA0)}
				i.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
			}},
		escenario{nombre: "reembolso", descripcion: "v1.1.0 de reembolso de gastos (Anexo 5): reembolsa la factura 113 de este mismo emisor",
			construir: func(f *factura.Factura) {
				compradorRUC(f)
				origen := facturaAutorizada("113-")
				oi := origen.InfoFactura
				r := factura.ReembolsoDetalle{
					TipoIdentificacionProveedorReembolso: comprobante.IDRuc,
					IdentificacionProveedorReembolso:     origen.InfoTributaria.RUC,
					CodPaisPagoProveedorReembolso:        "593",
					TipoProveedorReembolso:               "01",
					CodDocReembolso:                      comprobante.CodFactura,
					EstabDocReembolso:                    origen.InfoTributaria.Estab,
					PtoEmiDocReembolso:                   origen.InfoTributaria.PtoEmi,
					SecuencialDocReembolso:               origen.InfoTributaria.Secuencial,
					FechaEmisionDocReembolso:             oi.FechaEmision,
					NumeroAutorizacionDocReemb:           origen.InfoTributaria.ClaveAcceso,
				}
				for _, t := range oi.TotalConImpuestos {
					r.DetalleImpuestos = append(r.DetalleImpuestos, factura.ImpuestoReembolso{
						Codigo: t.Codigo, CodigoPorcentaje: t.CodigoPorcentaje, Tarifa: *t.Tarifa,
						BaseImponibleReembolso: t.BaseImponible, ImpuestoReembolso: t.Valor,
					})
				}
				f.Reembolsos = []factura.ReembolsoDetalle{r}
				f.Detalles = []factura.Detalle{factura.LineaIVA("REEMB", "REEMBOLSO DE GASTOS", d("1"), oi.ImporteTotal, comprobante.IVA0)}
				f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
			}},
	)
}

// facturaAutorizada lee de las evidencias una factura autorizada cuyo
// archivo empieza por el prefijo dado.
func facturaAutorizada(prefijo string) *factura.Factura {
	rutas, _ := filepath.Glob(filepath.Join("evidencia-privada", "qa-*", prefijo+"*.autorizado.xml"))
	if len(rutas) == 0 {
		panic("no hay evidencia de la factura " + prefijo)
	}
	doc, err := os.ReadFile(rutas[0])
	if err != nil {
		panic(err)
	}
	s := string(doc)
	a, b := strings.Index(s, "<![CDATA["), strings.LastIndex(s, "]]>")
	f, err := factura.Parse([]byte(s[a+9 : b]))
	if err != nil {
		panic(err)
	}
	return f
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
	retDesde := flag.Int("ret-desde", 0, "secuencial de comprobante de retención; activa el escenario de retención")
	ndDesde := flag.Int("nd-desde", 0, "secuencial de nota de débito; activa el escenario de nota de débito")
	ncDesde := flag.Int("nc-desde", 0, "primer secuencial de nota de crédito; activa los escenarios de nota de crédito")
	guiaDesde := flag.Int("guia-desde", 0, "primer secuencial de guía de remisión; activa los escenarios de guía (usa 2)")
	liqDesde := flag.Int("liq-desde", 0, "primer secuencial de liquidación de compra; activa los escenarios de liquidación (usa 2)")
	loteDesde := flag.Int("lote-desde", 0, "primer secuencial de factura para un envío por lote de 2 facturas; el lote usa la serie -lote-serie")
	loteSerie := flag.String("lote-serie", "001999", "serie (estab+ptoEmi) reservada para la clave del lote")
	loteSec := flag.Int("lote-sec", 1, "secuencial del lote")
	loteAlterar := flag.Bool("lote-alterar", false, "altera el segundo comprobante del lote después de firmarlo")
	ret1Desde := flag.Int("ret1-desde", 0, "secuencial de comprobante de retención para probar la versión 1.0.0")
	dir := flag.String("dir", "evidencia-privada", "directorio de evidencias (excluido de Git)")
	soloRegresion := flag.Bool("regresion", false, "sin conexión: vuelve a validar con el SDK actual los comprobantes ya autorizados que hay en -dir")
	flag.Parse()
	if *soloRegresion {
		if regresion(*dir) > 0 {
			os.Exit(1)
		}
		return
	}
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
		if *solo == "" && (e.nombre == "exportacion" || e.nombre == "reembolso") {
			continue
		}
		if *solo == "" && (*ncDesde > 0 || *ndDesde > 0 || *retDesde > 0 || *guiaDesde > 0 || *liqDesde > 0 || *loteDesde > 0 || *ret1Desde > 0) {
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
	if *retDesde > 0 {
		decir("\n== retencion (comprobante de retención %d)", *retDesde)
		it, err := entorno.Emisor(comprobante.CodComprobanteRetencion, *retDesde)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		hoy := comprobante.Hoy()
		c := &retencion.ComprobanteRetencion{
			InfoTributaria: it,
			InfoCompRetencion: retencion.InfoCompRetencion{
				FechaEmision:                     hoy,
				ObligadoContabilidad:             entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO"),
				TipoIdentificacionSujetoRetenido: comprobante.IDRuc,
				ParteRel:                         "NO",
				RazonSocialSujetoRetenido:        razonPruebas,
				IdentificacionSujetoRetenido:     rucSRI,
			},
			DocsSustento: []retencion.DocSustento{{
				CodSustento: "01", CodDocSustento: "01", NumDocSustento: "001001000004321",
				FechaEmisionDocSustento: hoy, PagoLocExt: retencion.PagoLocal,
				TotalSinImpuestos: d("200.00"),
				ImpuestosDocSustento: []retencion.ImpuestoDocSustento{
					{CodImpuestoDocSustento: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15, BaseImponible: d("200.00")},
				},
				Retenciones: []retencion.Retencion{
					{Codigo: retencion.ImpuestoRenta, CodigoRetencion: "312", BaseImponible: d("200.00"), PorcentajeRetener: d("2")},
					{Codigo: retencion.ImpuestoIVA, CodigoRetencion: "1", BaseImponible: d("30.00"), PorcentajeRetener: d("30")},
				},
				Pagos: []retencion.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}},
			}},
		}
		c.Calcular()
		c.GenerarClaveAcceso("")
		cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: it.RUC})
		if err != nil {
			panic(err)
		}
		if prep, err := cli.Preparar(c); err != nil {
			decir("   ERROR local: %v", err)
			fallos++
		} else {
			base := filepath.Join(ronda, fmt.Sprintf("ret-%d", *retDesde))
			os.WriteFile(base+".firmado.xml", prep.XMLFirmado, 0o600)
			decir("   local: reglas OK, XSD OK, firma verificada; v%s; renta 312 (2 %%) + IVA 30 %%; total retenido %s", retencion.Version, c.TotalRetenido())
			if !enviar(cli, prep.XMLFirmado, base, decir) {
				fallos++
			}
		}
	}
	if *guiaDesde > 0 {
		decir("\n== guia-remision (factura %d, guías %d y %d)", sec, *guiaDesde, *guiaDesde+1)
		emisor, err := entorno.Emisor(comprobante.CodFactura, sec)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		f := &factura.Factura{InfoTributaria: emisor}
		f.InfoFactura.FechaEmision = comprobante.Hoy()
		f.InfoFactura.ObligadoContabilidad = entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO")
		compradorRUC(f)
		f.Detalles = []factura.Detalle{factura.LineaIVA("M-1", "Mercadería para traslado", d("10.254632"), d("3.00"), comprobante.IVA15)}
		f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
		f.Calcular()
		f.GenerarClaveAcceso("")
		cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: emisor.RUC})
		if err != nil {
			panic(err)
		}
		sustento := false
		if prep, err := cli.Preparar(f); err != nil {
			decir("   ERROR local en la factura: %v", err)
			fallos++
		} else {
			sec++
			decir("   -- factura de sustento, importe %s", f.InfoFactura.ImporteTotal)
			sustento = enviar(cli, prep.XMLFirmado, filepath.Join(ronda, fmt.Sprintf("%d-factura-para-guia", sec-1)), decir)
			if !sustento {
				fallos++
			}
		}
		for i, ver := range []string{guiaremision.V110, guiaremision.V100} {
			it, _ := entorno.Emisor(comprobante.CodGuiaRemision, *guiaDesde+i)
			dest := guiaremision.Destinatario{
				IdentificacionDestinatario: rucSRI,
				RazonSocialDestinatario:    razonPruebas,
				DirDestinatario:            "Av. Amazonas y Unión Nacional de Periodistas",
				MotivoTraslado:             "Venta (prueba)",
				Ruta:                       "Quito – Cayambe – Otavalo",
				Detalles: []guiaremision.Detalle{
					{CodigoInterno: "M-1", Descripcion: "Mercadería para traslado", Cantidad: d("10.254632"),
						DetallesAdicionales: []comprobante.DetAdicional{{Nombre: "Lote", Valor: "L-77 & Ñ"}}},
					{Descripcion: "Pallet de madera", Cantidad: d("2")},
				},
			}
			descripcion := "sin comprobante de sustento, traslado entre establecimientos"
			if ver == guiaremision.V110 && sustento {
				dest.Sustento(comprobante.CodFactura, f.InfoTributaria.NumeroComprobante(), f.InfoTributaria.ClaveAcceso, f.InfoFactura.FechaEmision)
				descripcion = "con factura de sustento, cantidad con 6 decimales"
			} else {
				dest.MotivoTraslado = "Traslado entre establecimientos (prueba)"
				dest.CodEstabDestino = "001"
				dest.IdentificacionDestinatario, dest.RazonSocialDestinatario = it.RUC, it.RazonSocial
			}
			if ver == guiaremision.V100 {
				dest.Detalles[0].Cantidad = d("10.25")
			}
			g := &guiaremision.GuiaRemision{
				Version:        ver,
				InfoTributaria: it,
				InfoGuiaRemision: guiaremision.InfoGuiaRemision{
					DirPartida:                      it.DirMatriz,
					RazonSocialTransportista:        razonPruebas,
					TipoIdentificacionTransportista: comprobante.IDRuc,
					RucTransportista:                rucSRI,
					ObligadoContabilidad:            entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO"),
					FechaIniTransporte:              comprobante.Hoy(),
					FechaFinTransporte:              comprobante.FechaDe(time.Now().AddDate(0, 0, 1)),
					Placa:                           "PBA1234",
				},
				Destinatarios: []guiaremision.Destinatario{dest},
				InfoAdicional: []comprobante.CampoAdicional{{Nombre: "Observación", Valor: "Guía de prueba"}},
			}
			g.GenerarClaveAcceso("")
			decir("   -- guía de remisión %d, v%s: %s", *guiaDesde+i, ver, descripcion)
			prep, err := cli.Preparar(g)
			if err != nil {
				decir("   ERROR local en la guía: %v", err)
				fallos++
				continue
			}
			base := filepath.Join(ronda, fmt.Sprintf("guia-%d", *guiaDesde+i))
			os.WriteFile(base+".firmado.xml", prep.XMLFirmado, 0o600)
			if !enviar(cli, prep.XMLFirmado, base, decir) {
				fallos++
			}
		}
	}
	if *liqDesde > 0 {
		decir("\n== liquidacion-compra (liquidaciones %d y %d)", *liqDesde, *liqDesde+1)
		for i, ver := range []string{liquidacion.V110, liquidacion.V100} {
			it, err := entorno.Emisor(comprobante.CodLiquidacionCompra, *liqDesde+i)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			l := &liquidacion.LiquidacionCompra{
				Version:        ver,
				InfoTributaria: it,
				InfoLiquidacionCompra: liquidacion.InfoLiquidacionCompra{
					FechaEmision:         comprobante.Hoy(),
					ObligadoContabilidad: entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO"),
					// Proveedor: la cédula del propio emisor, con la razón
					// social de pruebas de la ficha (sección 5.6).
					TipoIdentificacionProveedor: comprobante.IDCedula,
					RazonSocialProveedor:        razonPruebas,
					IdentificacionProveedor:     it.RUC[:10],
					DireccionProveedor:          "Vía a la Costa km 12",
					Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}},
				},
				InfoAdicional: []comprobante.CampoAdicional{{Nombre: "Observación", Valor: "Liquidación de prueba"}},
			}
			descripcion := "proveedor con cédula, bien con IVA 0 % (6 decimales) y servicio con IVA 15 %"
			if ver == liquidacion.V110 {
				l.Detalles = []liquidacion.Detalle{
					liquidacion.LineaIVA("CACAO", "Cacao en grano", d("12.345678"), d("3.25"), comprobante.IVA0),
					liquidacion.LineaIVA("FLETE", "Servicio de acarreo", d("1"), d("20.00"), comprobante.IVA15),
				}
				l.Detalles[0].UnidadMedida = "quintal"
			} else {
				descripcion = "un servicio con IVA 15 % y descuento"
				l.Detalles = []liquidacion.Detalle{liquidacion.LineaIVA("SRV", "Servicio ocasional", d("2"), d("15.50"), comprobante.IVA15)}
				l.Detalles[0].Descuento = d("1.00")
			}
			l.Calcular()
			l.GenerarClaveAcceso("")
			decir("   -- liquidación %d, v%s: %s; importe %s", *liqDesde+i, ver, descripcion, l.InfoLiquidacionCompra.ImporteTotal)
			cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: it.RUC})
			if err != nil {
				panic(err)
			}
			prep, err := cli.Preparar(l)
			if err != nil {
				decir("   ERROR local en la liquidación: %v", err)
				fallos++
				continue
			}
			for _, h := range prep.Advertencias {
				decir("   advertencia local: %s", h)
			}
			base := filepath.Join(ronda, fmt.Sprintf("liq-%d", *liqDesde+i))
			os.WriteFile(base+".firmado.xml", prep.XMLFirmado, 0o600)
			if !enviar(cli, prep.XMLFirmado, base, decir) {
				fallos++
			}
		}
	}
	if *loteDesde > 0 {
		decir("\n== lote (facturas %d y %d, lote %s-%d)", *loteDesde, *loteDesde+1, *loteSerie, *loteSec)
		var docs [][]byte
		var emisor comprobante.InfoTributaria
		for i := 0; i < 2; i++ {
			it, err := entorno.Emisor(comprobante.CodFactura, *loteDesde+i)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			emisor = it
			f := &factura.Factura{InfoTributaria: it}
			f.InfoFactura.FechaEmision = comprobante.Hoy()
			f.InfoFactura.ObligadoContabilidad = entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO")
			consumidorFinal(f)
			f.Calcular()
			f.GenerarClaveAcceso("")
			cli, _ := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: it.RUC})
			prep, err := cli.Preparar(f)
			if err != nil {
				decir("   ERROR local: %v", err)
				os.Exit(1)
			}
			os.WriteFile(filepath.Join(ronda, fmt.Sprintf("lote-factura-%d.firmado.xml", *loteDesde+i)), prep.XMLFirmado, 0o600)
			docs = append(docs, prep.XMLFirmado)
		}
		if *loteAlterar {
			docs[1] = []byte(strings.Replace(string(docs[1]), "PRODUCTO DE PRUEBA", "PRODUCTO ALTERADO", 1))
			decir("   el segundo comprobante se alteró después de firmarlo")
		}
		claveLote, err := clave.Generar(clave.Componentes{
			FechaEmision: comprobante.Hoy().Time(), TipoComprobante: comprobante.CodFactura, RUC: emisor.RUC, Ambiente: "1",
			Establecimiento: (*loteSerie)[:3], PuntoEmision: (*loteSerie)[3:], Secuencial: fmt.Sprintf("%09d", *loteSec), CodigoNumerico: "12345678",
		})
		if err != nil {
			panic(err)
		}
		servicios := ws.Nuevo(comprobante.Pruebas)
		ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancelar()
		rec, err := servicios.ValidarLote(ctx, ws.Lote{ClaveAcceso: claveLote, RUC: emisor.RUC, Comprobantes: docs})
		if err != nil {
			decir("   ERROR de comunicación: %v", err)
		} else {
			decir("   recepción del lote: %s", rec.Estado)
			for _, m := range rec.Mensajes() {
				decir("     %s %s", m.Tipo, sanear(m.String()))
			}
			for intento := 0; intento < 8 && rec.Recibida(); intento++ {
				time.Sleep(4 * time.Second)
				rl, err := servicios.AutorizacionLote(ctx, claveLote)
				if err != nil {
					decir("   ERROR al consultar el lote: %v", err)
					break
				}
				decir("   consulta %d: comprobantes del lote = %q, autorizaciones = %d", intento+1, rl.NumeroComprobantesLote, len(rl.Autorizaciones))
				for _, a := range rl.Autorizaciones {
					decir("     estado %q (secuencial %s)", a.Estado, secuencialDe(a.Comprobante))
					for _, m := range a.Mensajes {
						decir("       %s %s", m.Tipo, sanear(m.String()))
					}
				}
				if len(rl.Autorizaciones) >= len(docs) {
					break
				}
			}
		}
	}
	if *ret1Desde > 0 {
		decir("\n== retencion-1.0.0 (comprobante de retención %d)", *ret1Desde)
		it, err := entorno.Emisor(comprobante.CodComprobanteRetencion, *ret1Desde)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		hoy := comprobante.Hoy()
		c := &retencion.ComprobanteV1{
			InfoTributaria: it,
			InfoCompRetencion: retencion.InfoV1{
				FechaEmision:                     hoy,
				ObligadoContabilidad:             entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO"),
				TipoIdentificacionSujetoRetenido: comprobante.IDRuc,
				RazonSocialSujetoRetenido:        razonPruebas,
				IdentificacionSujetoRetenido:     rucSRI,
			},
			Impuestos: []retencion.ImpuestoV1{
				{Codigo: retencion.ImpuestoRenta, CodigoRetencion: "312", BaseImponible: d("200.00"), PorcentajeRetener: d("2"),
					CodDocSustento: "01", NumDocSustento: "001001000004321", FechaEmisionDocSustento: &hoy},
				{Codigo: retencion.ImpuestoIVA, CodigoRetencion: "1", BaseImponible: d("30.00"), PorcentajeRetener: d("30"),
					CodDocSustento: "01", NumDocSustento: "001001000004321", FechaEmisionDocSustento: &hoy},
			},
		}
		c.Calcular()
		c.GenerarClaveAcceso("")
		cli, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: it.RUC})
		if err != nil {
			panic(err)
		}
		if prep, err := cli.Preparar(c); err != nil {
			decir("   ERROR local: %v", err)
			fallos++
		} else {
			base := filepath.Join(ronda, fmt.Sprintf("ret1-%d", *ret1Desde))
			os.WriteFile(base+".firmado.xml", prep.XMLFirmado, 0o600)
			decir("   local: reglas OK, XSD OK, firma verificada; v%s; total retenido %s", retencion.VersionV1, c.TotalRetenido())
			if !enviar(cli, prep.XMLFirmado, base, decir) {
				fallos++
			}
		}
	}
	decir("\nsecuenciales de factura consumidos: %d (desde %d); de nota de crédito: %d; escenarios no autorizados: %d", sec-*desde, *desde, secNC-*ncDesde, fallos)
	decir("evidencias: %s", ronda)
}

// secuencialDe extrae el secuencial de un comprobante, para distinguir los
// resultados de un lote sin mostrar datos del emisor.
func secuencialDe(doc string) string {
	a, b := strings.Index(doc, "<secuencial>"), strings.Index(doc, "</secuencial>")
	if a < 0 || b < a {
		return "?"
	}
	return doc[a+12 : b]
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

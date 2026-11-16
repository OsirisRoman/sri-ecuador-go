// Comando certificacion contrasta con el ambiente de PRUEBAS del SRI las
// reglas y los estados que cambiaron en la versión 0.2.0 (auditoría de
// v0.1.0): bases de IVA, redondeo, reenvío corregido con la misma clave
// (ficha 5.10), estados intermedios y conservación de respuestas.
//
// Sin -enviar no se comunica con el SRI: construye cada comprobante, aplica
// las validaciones locales y escribe el resultado esperado. Con -enviar,
// además lo envía. Nunca opera contra producción (entorno.Emisor fija el
// ambiente de pruebas y el cliente se crea con comprobante.Pruebas).
//
//	go run ./internal/qa/certificacion -desde 140             # predicción
//	go run ./internal/qa/certificacion -desde 140 -enviar     # envío
//	go run ./internal/qa/certificacion -desde 140 -solo T1 -enviar
//
// # Procedimiento aislado para comprobantes incorrectos
//
// Las pruebas T1 y T5 envían a propósito un comprobante que el SDK rechaza
// (base de IVA incoherente; un centavo de más), para saber si el SRI también
// lo rechaza. No se modifica ni se desactiva ninguna validación del SDK:
// este comando comprueba primero que Validar da exactamente el error
// esperado, valida el XSD, firma con xades.Firmar y envía con
// Cliente.Reanudar, que no valida. Solo existe en este comando de
// mantenimiento, en el ambiente de pruebas.
//
// La salida no incluye datos del emisor; los XML y las respuestas se
// guardan en -dir (evidencia-privada/, excluido de Git).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/entorno"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
	"github.com/OsirisRoman/sri-ecuador-go/ride"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

var (
	ncSec     int
	enviarSRI bool
	cert      *certificado.Certificado
	ronda     string
	decir     func(string, ...any)
)

func main() {
	desde := flag.Int("desde", 0, "primer secuencial de factura libre (obligatorio)")
	solo := flag.String("solo", "", "pruebas a ejecutar, separadas por comas (T0..T9); por defecto, todas")
	flag.BoolVar(&enviarSRI, "enviar", false, "enviar al ambiente de pruebas del SRI (sin esto, solo predicción local)")
	dir := flag.String("dir", "evidencia-privada", "directorio de evidencias (excluido de Git)")
	flag.IntVar(&ncSec, "nc", 0, "secuencial de nota de crédito para T7 (nota de crédito 1.0.0)")
	flag.Parse()
	if *desde < 1 {
		fmt.Fprintln(os.Stderr, "indique -desde con el primer secuencial de factura libre")
		os.Exit(2)
	}
	entorno.CargarEnv(".env")
	var err error
	if cert, err = entorno.Certificado(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	ronda = filepath.Join(*dir, "certificacion-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(ronda, 0o700); err != nil {
		panic(err)
	}
	resumen, _ := os.OpenFile(filepath.Join(ronda, "resumen.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	defer resumen.Close()
	decir = func(formato string, args ...any) {
		linea := sanear(fmt.Sprintf(formato, args...))
		fmt.Println(linea)
		fmt.Fprintln(resumen, linea)
	}
	decir("Ronda de certificación (envío=%t). Certificado vigente hasta %s.", enviarSRI, cert.Resumen().Hasta.Format("2006-01-02"))

	sec := *desde
	pruebas := []struct {
		id  string
		usa int // secuenciales de factura que consume
		fn  func(sec int)
	}{
		{"T0", 0, t0Conexion},
		{"T1", 1, t1BaseIncoherente},
		{"T2", 1, t2SinIVA},
		{"T3", 1, t3SumaDeLineas},
		{"T4", 1, t4MitadAlPar},
		{"T5", 1, t5CentavoDeMas},
		{"T6", 0, t6ClaveNoEnviada},
		{"T7", 0, t7NotaCredito100},
		{"T8", 1, t8PrecioFinal},
		{"T9", 1, t9PrecioFinalVariasLineas},
	}
	for _, p := range pruebas {
		if *solo == "" || strings.Contains(","+*solo+",", ","+p.id+",") {
			p.fn(sec)
		}
		sec += p.usa
	}
}

// --- Utilidades -------------------------------------------------------------

func cliente(espera sri.Espera) *sri.Cliente {
	c, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, RUC: os.Getenv("SRI_EMISOR_RUC"), Espera: espera})
	if err != nil {
		panic(err)
	}
	return c
}

// facturaBase devuelve una factura al comprador de pruebas de la ficha
// (sección 5.6), con las líneas dadas, calculada y con clave.
func facturaBase(sec int, lineas ...factura.Detalle) *factura.Factura {
	emisor, err := entorno.Emisor(comprobante.CodFactura, sec)
	if err != nil {
		panic(err)
	}
	f := &factura.Factura{InfoTributaria: emisor, Detalles: lineas}
	i := &f.InfoFactura
	i.FechaEmision = comprobante.Hoy()
	i.DirEstablecimiento = entorno.Valor("SRI_EMISOR_DIR_ESTABLECIMIENTO", emisor.DirMatriz)
	i.ObligadoContabilidad = entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO")
	i.TipoIdentificacionComprador = comprobante.IDRuc
	i.RazonSocialComprador = "PRUEBAS SERVICIO DE RENTAS INTERNAS"
	i.IdentificacionComprador = "1760013210001"
	i.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
	f.Calcular()
	if _, err := f.GenerarClaveAcceso(""); err != nil {
		panic(err)
	}
	return f
}

// reglas resume los hallazgos locales.
func reglas(hs comprobante.Hallazgos) string {
	var out []string
	for _, h := range hs {
		out = append(out, h.Severidad.String()+" "+h.Regla)
	}
	if len(out) == 0 {
		return "ninguno"
	}
	return strings.Join(out, ", ")
}

func guardar(nombre string, datos []byte) { os.WriteFile(filepath.Join(ronda, nombre), datos, 0o600) }

// firmarSinValidar es el procedimiento aislado para comprobantes que el SDK
// rechaza: exige el XSD y firma, sin pasar por Preparar.
func firmarSinValidar(f *factura.Factura) []byte {
	doc, err := f.XML()
	if err != nil {
		panic(err)
	}
	if err := esquema.Validar(doc); err != nil {
		panic(fmt.Sprintf("el caso de prueba no cumple el XSD: %v", err))
	}
	firmado, err := xades.Firmar(doc, cert, nil)
	if err != nil {
		panic(err)
	}
	return firmado
}

// clasificar describe el error del SDK por su tipo.
func clasificar(err error) string {
	switch {
	case err == nil:
		return "sin error"
	case errors.Is(err, sri.ErrDiscrepancia):
		return "ErrDiscrepancia"
	case errors.Is(err, sri.ErrEstadoIndeterminado):
		return "ErrEstadoIndeterminado"
	case errors.Is(err, sri.ErrRespuestaIncoherente):
		return "ErrRespuestaIncoherente"
	}
	return "error: " + err.Error()
}

// informar escribe el resultado y conserva lo que la aplicación debe
// guardar (objetivo 5): XML firmado, autorizado, resultado y RIDE.
func informar(nombre string, res *sri.Resultado, err error) {
	if res == nil {
		decir("   resultado: nil; %s", clasificar(err))
		return
	}
	if rec := res.Recepcion; rec != nil {
		decir("   recepción: %q", rec.Estado)
		for _, m := range rec.Mensajes() {
			decir("     %s %s", m.Tipo, m)
		}
	}
	decir("   SDK: estado=%s verificado=%t %s", res.Estado, res.Verificado, clasificar(err))
	if a := res.Autorizacion; a != nil {
		decir("   SRI: estado=%q ambiente=%q fecha=%q", a.Estado, a.Ambiente, a.FechaAutorizacion)
		for _, m := range a.Mensajes {
			decir("     %s %s", m.Tipo, m)
		}
	}
	conservado := map[string]any{
		"estado": res.Estado.String(), "verificado": res.Verificado, "error": clasificar(err),
		"claveAccesoGuardada": res.ClaveAcceso != "", "xmlFirmadoBytes": len(res.XMLFirmado),
		"mensajes": res.Mensajes,
	}
	if x := res.XMLAutorizado(); x != nil {
		guardar(nombre+".autorizado.xml", x)
		pdf, errRide := ride.Generar(x, nil)
		conservado["xmlAutorizadoBytes"], conservado["rideBytes"], conservado["rideError"] = len(x), len(pdf), fmt.Sprint(errRide)
		decir("   conservado: XML autorizado (%d bytes) y RIDE (%d bytes, error=%v)", len(x), len(pdf), errRide)
	}
	j, _ := json.MarshalIndent(conservado, "", "  ")
	guardar(nombre+".resultado.json", j)
}

// consultaCruda consulta la autorización sin interpretar y describe lo que
// devuelve el SRI (objetivo 4): número de autorizaciones, estados, clave
// consultada, ambiente y si cada comprobante devuelto es el enviado.
func consultaCruda(cli *sri.Cliente, claveAcceso string, enviado []byte, etiqueta string) {
	ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
	defer cancelar()
	r, err := cli.Servicios().AutorizacionComprobante(ctx, claveAcceso)
	if err != nil {
		decir("   consulta %s: error %v", etiqueta, err)
		return
	}
	huella, _ := xades.Huella(enviado)
	var partes []string
	for _, a := range r.Autorizaciones {
		mismo := "sin comprobante"
		if strings.TrimSpace(a.Comprobante) != "" {
			h, err := xades.Huella([]byte(strings.TrimSpace(a.Comprobante)))
			mismo = fmt.Sprintf("¿es el enviado? %t", err == nil && h == huella)
		}
		partes = append(partes, fmt.Sprintf("{estado=%q ambiente=%q %s}", a.Estado, a.Ambiente, mismo))
	}
	decir("   consulta %s: claveAccesoConsultada coincide=%t numeroComprobantes=%q autorizaciones=%d %s",
		etiqueta, strings.TrimSpace(r.ClaveAccesoConsultada) == claveAcceso, r.NumeroComprobantes, len(r.Autorizaciones), strings.Join(partes, " "))
}

func contexto() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 4*time.Minute)
}

// --- Pruebas ----------------------------------------------------------------

// T0: conectividad, sin emitir: consulta un comprobante ya autorizado.
func t0Conexion(int) {
	decir("\n== T0 conectividad y lectura de una autorización existente (sin emitir)")
	rutas, _ := filepath.Glob(filepath.Join("evidencia-privada", "qa-*", "113-*.firmado.xml"))
	if len(rutas) == 0 {
		decir("   sin evidencia de la factura 113; se omite")
		return
	}
	firmado, _ := os.ReadFile(rutas[0])
	decir("   esperado: AUTORIZADO, verificado=true (la factura 113 se autorizó el 2026-11-14)")
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	res, err := cliente(sri.Espera{}).ConsultarDocumento(ctx, firmado)
	informar("T0", res, err)
}

// T1: base de IVA incoherente (AUD-001) y, si el SRI no la autoriza,
// corrección con la misma clave y secuencial (ficha 5.10; AUD-002).
func t1BaseIncoherente(sec int) {
	decir("\n== T1 base de IVA incoherente (factura %d) y reenvío corregido con la misma clave", sec)
	f := facturaBase(sec, factura.LineaIVA("T1", "Servicio de prueba", d("1"), d("100.00"), comprobante.IVA15))
	imp := &f.Detalles[0].Impuestos[0]
	imp.BaseImponible, imp.Valor = d("1.00"), d("0.15")
	t := &f.InfoFactura.TotalConImpuestos[0]
	t.BaseImponible, t.Valor = d("1.00"), d("0.15")
	f.InfoFactura.ImporteTotal = d("100.15")
	f.InfoFactura.Pagos[0].Total = d("100.15")
	hs := f.Validar()
	decir("   local: %s", reglas(hs))
	if len(hs.Errores()) != 1 || !hs.Tiene("iva.base") {
		decir("   ABORTADO: la validación local no da exactamente el error iva.base")
		return
	}
	decir("   esperado SDK: Preparar lo rechaza (iva.base). Se envía con el procedimiento aislado.")
	decir("   hipótesis SRI: NO AUTORIZADO con el código 52 (error en diferencias); no comprobado antes")
	incorrecto := firmarSinValidar(f)
	guardar("T1-incorrecto.firmado.xml", incorrecto)

	// Corrección: mismos datos con el IVA correcto, misma clave.
	g := facturaBase(sec, factura.LineaIVA("T1", "Servicio de prueba", d("1"), d("100.00"), comprobante.IVA15))
	g.InfoTributaria.ClaveAcceso = f.InfoTributaria.ClaveAcceso
	cli := cliente(sri.Espera{})
	prep, err := cli.Preparar(g)
	if err != nil {
		decir("   ABORTADO: la corrección no pasa Preparar: %v", err)
		return
	}
	decir("   corrección: Preparar OK, misma clave=%t, importe %s", g.InfoTributaria.ClaveAcceso == f.InfoTributaria.ClaveAcceso, g.InfoFactura.ImporteTotal)
	guardar("T1-corregido.firmado.xml", prep.XMLFirmado)
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	res, err := cli.Reanudar(ctx, incorrecto, nil)
	decir("   -- envío del incorrecto")
	informar("T1-incorrecto", res, err)
	if res == nil || (res.Estado != sri.EstadoNoAutorizado && res.Estado != sri.EstadoDevuelto) {
		decir("   el SRI no lo rechazó: no se reenvía la corrección (el secuencial no es reutilizable)")
		return
	}
	decir("   -- reenvío corregido con la misma clave y secuencial (5.10)")
	decir("   esperado: recepción RECIBIDA; mientras se procesa, el NO AUTORIZADO anterior no cuenta; final AUTORIZADO, verificado=true")
	r2, err := cli.Enviar(ctx, prep.XMLFirmado)
	if err != nil || r2 == nil || r2.Recepcion == nil {
		decir("   recepción: %s", clasificar(err))
		return
	}
	decir("   recepción del corregido: %q", r2.Recepcion.Estado)
	for _, m := range r2.Recepcion.Mensajes() {
		decir("     %s %s", m.Tipo, m)
	}
	// Lo que devuelve el SRI mientras procesa la corrección.
	for i, espera := range []time.Duration{0, 2 * time.Second, 4 * time.Second, 8 * time.Second} {
		time.Sleep(espera)
		consultaCruda(cli, g.InfoTributaria.ClaveAcceso, prep.XMLFirmado, fmt.Sprintf("%d (+%s)", i+1, espera))
	}
	res, err = cli.ConsultarDocumento(ctx, prep.XMLFirmado)
	decir("   -- ConsultarDocumento del corregido")
	informar("T1-corregido", res, err)
	// Reenvío idéntico del autorizado: 43 y verificado (AUD-002, sin discrepancia).
	decir("   -- reenvío idéntico del corregido (mismo documento): esperado 43 y AUTORIZADO verificado")
	res, err = cli.Reanudar(ctx, prep.XMLFirmado, nil)
	informar("T1-reenvio", res, err)
}

// T2: detalle sin IVA (advertencia iva.ausente): una línea solo con IRBPNR.
func t2SinIVA(sec int) {
	decir("\n== T2 detalle sin IVA (factura %d): línea con IRBPNR y sin IVA, más una línea con IVA", sec)
	sinIVA := factura.Detalle{CodigoPrincipal: "T2-B", Descripcion: "Botella plástica (IRBPNR)", Cantidad: d("10"), PrecioUnitario: d("0.50"),
		Impuestos: []comprobante.Impuesto{{Codigo: comprobante.ImpuestoIRBPNR, CodigoPorcentaje: "5001", Tarifa: d("0.02"), Valor: d("0.20")}}}
	f := facturaBase(sec, factura.LineaIVA("T2-A", "Servicio de prueba", d("1"), d("10.00"), comprobante.IVA15), sinIVA)
	cli := cliente(sri.Espera{})
	prep, err := cli.Preparar(f)
	decir("   local: %s", reglas(f.Validar()))
	if err != nil {
		decir("   ABORTADO: %v", err)
		return
	}
	decir("   esperado SDK: Preparar lo admite con la advertencia iva.ausente")
	decir("   hipótesis SRI: desconocida; podría rechazarlo por falta de IVA o por el IRBPNR (un rechazo por el IRBPNR no prueba nada sobre el IVA)")
	guardar("T2.firmado.xml", prep.XMLFirmado)
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	res, err := cli.Reanudar(ctx, prep.XMLFirmado, prep.Advertencias)
	informar("T2", res, err)
}

// lineas devuelve n líneas de 0,10 al 15 %, cuyo IVA exacto es 0,015.
func lineas(n int) []factura.Detalle {
	var ls []factura.Detalle
	for i := 0; i < n; i++ {
		ls = append(ls, factura.LineaIVA(fmt.Sprintf("R-%02d", i+1), "Artículo de prueba de redondeo", d("1"), d("0.10"), comprobante.IVA15))
	}
	return ls
}

// redondeo emite una factura de 10 líneas de 0,10 con el IVA de cada línea
// y el total dados, observando el estado intermedio (objetivo 4) y la
// recuperación de un pendiente con ConsultarDocumento (objetivo 5).
func redondeo(id string, sec int, linea, total, explicacion string) {
	f := facturaBase(sec, lineas(10)...)
	for i := range f.Detalles {
		f.Detalles[i].Impuestos[0].Valor = d(linea)
	}
	f.InfoFactura.TotalConImpuestos[0].Valor = d(total)
	f.InfoFactura.ImporteTotal = f.InfoFactura.TotalSinImpuestos.Sumar(d(total))
	f.InfoFactura.Pagos[0].Total = f.InfoFactura.ImporteTotal
	decir("   %s", explicacion)
	cli := cliente(sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 1})
	prep, err := cli.Preparar(f)
	decir("   local: %s", reglas(f.Validar()))
	if err != nil {
		decir("   ABORTADO: %v", err)
		return
	}
	decir("   esperado SDK: Preparar lo admite (decisión D5)")
	guardar(id+".firmado.xml", prep.XMLFirmado)
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	r, err := cli.Enviar(ctx, prep.XMLFirmado)
	if err != nil {
		decir("   recepción: %s", clasificar(err))
		return
	}
	decir("   recepción: %q", r.Recepcion.Estado)
	if r.Estado != sri.EstadoRecibido {
		informar(id, r, nil)
		return
	}
	consultaCruda(cli, r.ClaveAcceso, prep.XMLFirmado, "inmediata")
	// Recuperación de un pendiente: ConsultarDocumento hasta un resultado.
	for i := 0; i < 6; i++ {
		res, err := cli.ConsultarDocumento(ctx, prep.XMLFirmado)
		if err != nil || !res.Pendiente() {
			decir("   ConsultarDocumento tras %d intento(s):", i+1)
			informar(id, res, err)
			return
		}
		time.Sleep(4 * time.Second)
	}
	decir("   sigue pendiente tras 6 consultas")
}

// T3: total de IVA por suma de líneas, distinto de la base acumulada.
func t3SumaDeLineas(sec int) {
	decir("\n== T3 IVA total por suma de líneas (factura %d)", sec)
	decir("   hipótesis SRI: desconocida; el SRI solo autorizó antes totales sobre la base acumulada")
	redondeo("T3", sec, "0.02", "0.20", "10 líneas: IVA por línea 0,015 → 0,02 (mitad hacia arriba); total 0,20 = suma de líneas (la base acumulada daría 0,15)")
}

// T4: IVA por línea redondeado mitad al par, total sobre la base acumulada.
func t4MitadAlPar(sec int) {
	decir("\n== T4 IVA por línea con desempate mitad al par (factura %d)", sec)
	decir("   hipótesis SRI: desconocida")
	redondeo("T4", sec, "0.01", "0.15", "10 líneas: IVA por línea 0,015 → 0,01 (mitad al par); total 0,15 = base acumulada")
}

// T5: un centavo de más en el IVA de una línea (AUD-003), procedimiento
// aislado.
func t5CentavoDeMas(sec int) {
	decir("\n== T5 IVA de una línea con un centavo de más (factura %d)", sec)
	f := facturaBase(sec, factura.LineaIVA("T5", "Servicio de prueba", d("1"), d("1.00"), comprobante.IVA15))
	f.Detalles[0].Impuestos[0].Valor = d("0.16")
	f.InfoFactura.TotalConImpuestos[0].Valor = d("0.16")
	f.InfoFactura.ImporteTotal = d("1.16")
	f.InfoFactura.Pagos[0].Total = d("1.16")
	hs := f.Validar()
	decir("   local: %s", reglas(hs))
	if !hs.Tiene("iva.valor") {
		decir("   ABORTADO: la validación local no da iva.valor")
		return
	}
	decir("   esperado SDK: Preparar lo rechaza (iva.valor). Se envía con el procedimiento aislado.")
	decir("   hipótesis SRI: NO AUTORIZADO 52; v0.1.0 lo aceptaba con su tolerancia de 0,01")
	firmado := firmarSinValidar(f)
	guardar("T5.firmado.xml", firmado)
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	res, err := cliente(sri.Espera{}).Reanudar(ctx, firmado, nil)
	informar("T5", res, err)
}

// T6: consulta de una clave que nunca se envió (no consume secuencial: el
// 999000001 no se usará).
func t6ClaveNoEnviada(int) {
	decir("\n== T6 consulta de una clave de acceso nunca enviada (sin emitir)")
	f := facturaBase(999000001, factura.LineaIVA("T6", "No se envía", d("1"), d("1.00"), comprobante.IVA15))
	decir("   esperado SDK: EstadoRecibido (pendiente) sin error si el SRI responde sin autorizaciones")
	if !enviarSRI {
		return
	}
	cli := cliente(sri.Espera{})
	consultaCruda(cli, f.InfoTributaria.ClaveAcceso, nil, "cruda")
	ctx, cancelar := contexto()
	defer cancelar()
	res, err := cli.Autorizar(ctx, f.InfoTributaria.ClaveAcceso)
	informar("T6", res, err)
}

// T7: nota de crédito 1.0.0 (nunca contrastada con el SRI) sobre la
// factura 141, asociada a la factura y a la nota de crédito 114 ya emitida
// sobre ella, para que las reglas origen.* acumulen ambas.
func t7NotaCredito100(int) {
	decir("\n== T7 nota de crédito 1.0.0 (nota de crédito %d) sobre la factura 141, con la nota 114 como anterior", ncSec)
	if ncSec < 1 {
		decir("   sin -nc; se omite")
		return
	}
	leer := func(patron string) []byte {
		rutas, _ := filepath.Glob(filepath.Join("evidencia-privada", "qa-*", patron))
		if len(rutas) != 1 {
			panic("evidencia ambigua o ausente: " + patron)
		}
		b, _ := os.ReadFile(rutas[0])
		if i := strings.Index(string(b), "<![CDATA["); i >= 0 {
			b = b[i+9 : strings.LastIndex(string(b), "]]>")]
		}
		return b
	}
	f, err := factura.Parse(leer("141-factura-para-nc.autorizado.xml"))
	if err != nil {
		panic(err)
	}
	anterior, err := notacredito.Parse(leer("nc-114-nota-credito-parcial.firmado.xml"))
	if err != nil {
		panic(err)
	}
	n := notacredito.DesdeFactura(f, "Devolución parcial de prueba (v1.0.0)")
	n.Version = notacredito.V100
	n.InfoTributaria.Secuencial = fmt.Sprintf("%09d", ncSec)
	var linea []notacredito.Detalle
	for _, det := range n.Detalles {
		if det.CodigoInterno == "B-2" {
			det.Cantidad = d("1")
			linea = append(linea, det)
		}
	}
	n.Detalles = linea
	n.Calcular()
	if _, err := n.GenerarClaveAcceso(""); err != nil {
		panic(err)
	}
	n.AsociarFactura(f, anterior)
	decir("   local: %s; valor %s", reglas(n.Validar()), n.InfoNotaCredito.ValorModificacion)
	// Contraprueba local (no se envía): con una anterior que ya devolvió
	// toda la factura, las reglas origen.* deben rechazarla.
	total := notacredito.DesdeFactura(f, "total")
	n.AsociarFactura(f, anterior, total)
	decir("   contraprueba local con una nota anterior total: %s", reglas(n.Validar()))
	n.AsociarFactura(f, anterior)
	cli := cliente(sri.Espera{})
	prep, err := cli.Preparar(n)
	if err != nil {
		decir("   ABORTADO: %v", err)
		return
	}
	decir("   esperado: AUTORIZADO, verificado=true")
	guardar("T7.firmado.xml", prep.XMLFirmado)
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	res, err := cli.Reanudar(ctx, prep.XMLFirmado, prep.Advertencias)
	informar("T7", res, err)
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
	return sb.String()
}

var _ = ws.EstadoAutorizado

// precioFinal emite por el flujo normal (Preparar) una factura cuyos
// detalles se definen por su precio final con IVA incluido, la consulta
// hasta tener resultado y comprueba que la factura leída de su XML se valida
// al declarar de nuevo el modo.
func precioFinal(id string, sec int, explicacion string, lineas ...factura.Detalle) {
	f := facturaBase(sec, lineas...)
	info := f.InfoFactura
	decir("   %s", explicacion)
	decir("   calculado: subtotal %s, IVA %s, importe total %s", info.TotalSinImpuestos, info.TotalConImpuestos[0].Valor, info.ImporteTotal)
	cli := cliente(sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 1})
	prep, err := cli.Preparar(f)
	decir("   local: %s", reglas(f.Validar()))
	if err != nil {
		decir("   ABORTADO: %v", err)
		return
	}
	leida, err := factura.Parse(prep.XMLFirmado)
	if err != nil {
		decir("   ABORTADO: %v", err)
		return
	}
	sinContexto := reglas(leida.Validar().Errores())
	leida.DeclararPreciosFinales()
	decir("   leída de su XML: sin declarar el modo, %s; con DeclararPreciosFinales, %s", sinContexto, reglas(leida.Validar().Errores()))
	decir("   esperado SDK: Preparar lo admite (precio final declarado)")
	decir("   hipótesis SRI: AUTORIZADO (autorizó un centavo de diferencia en T5); no comprobado con este cálculo")
	guardar(id+".firmado.xml", prep.XMLFirmado)
	if !enviarSRI {
		return
	}
	ctx, cancelar := contexto()
	defer cancelar()
	r, err := cli.Enviar(ctx, prep.XMLFirmado)
	if err != nil {
		decir("   recepción: %s", clasificar(err))
		return
	}
	decir("   recepción: %q", r.Recepcion.Estado)
	if r.Estado != sri.EstadoRecibido {
		informar(id, r, nil)
		return
	}
	for i := 0; i < 8; i++ {
		res, err := cli.ConsultarDocumento(ctx, prep.XMLFirmado)
		if err != nil || !res.Pendiente() {
			decir("   ConsultarDocumento tras %d intento(s):", i+1)
			informar(id, res, err)
			return
		}
		time.Sleep(4 * time.Second)
	}
	decir("   sigue pendiente tras 8 consultas")
}

// T8: una línea de precio final 12,00 con IVA del 15 %: base 10,43 e IVA
// 1,57 (el IVA de la base sería 1,5645 → 1,56).
func t8PrecioFinal(sec int) {
	decir("\n== T8 precio final con IVA incluido, una línea (factura %d)", sec)
	precioFinal("T8", sec, "1 × 12,00 de precio final: base 10,43 + IVA 1,57 = 12,00 (sobre la base, el IVA sería 1,56)",
		factura.LineaPrecioFinal("T8", "Servicio de prueba", d("1"), d("12.00"), comprobante.IVA15))
}

// T9: diez líneas de precio final 0,50: base 0,43 e IVA 0,07 cada una;
// totales 4,30 + 0,70 = 5,00 (sobre la base acumulada el IVA sería 0,65).
func t9PrecioFinalVariasLineas(sec int) {
	decir("\n== T9 precio final con IVA incluido, diez líneas (factura %d)", sec)
	var ls []factura.Detalle
	for i := 1; i <= 10; i++ {
		ls = append(ls, factura.LineaPrecioFinal(fmt.Sprintf("T9-%02d", i), "Artículo de prueba", d("1"), d("0.50"), comprobante.IVA15))
	}
	precioFinal("T9", sec, "10 líneas de 0,50 de precio final: 0,43 + 0,07 cada una; totales 4,30 + 0,70 = 5,00 (sobre la base acumulada, el IVA sería 0,65)", ls...)
}

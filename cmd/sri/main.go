// Comando sri: utilidades de línea de comandos del SDK. Sirve para
// comprobar un certificado, validar, firmar y verificar comprobantes, y para
// ejecutar pruebas reales contra el ambiente de certificación del SRI.
//
// La configuración privada se toma de variables de entorno o de un archivo
// .env (ver .env.example). La contraseña del certificado nunca se pasa por
// argumentos de línea de comandos.
package main

import (
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/correo"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/entorno"
	"github.com/OsirisRoman/sri-ecuador-go/ride"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

const uso = `Uso: sri <orden> [opciones]

Órdenes sin conexión:
  certificado              Comprueba el .p12 configurado y muestra datos no sensibles
  validar  <archivo.xml>   Valida un comprobante contra el XSD oficial
  firmar   <archivo.xml>   Firma un comprobante (XAdES-BES) y lo escribe en -salida
  verificar <archivo.xml>  Verifica la firma de un comprobante

Órdenes que se comunican con el SRI (solo ambiente de PRUEBAS):
  factura-prueba           Genera una factura mínima de prueba; con -enviar la envía
  enviar   <firmado.xml>   Envía un comprobante firmado y espera su autorización
  autorizacion <clave>     Consulta la autorización de una clave de acceso
  estado <clave>           Consulta de validez (AUTORIZADO, ANULADO, …)

Representación impresa:
  ride <comprobante.xml>   Genera el RIDE en PDF (-salida, -logo); use el .autorizado.xml

Correo (opcional; variables SMTP_*):
  correo <autorizado.xml>  Compone el correo de entrega (XML y RIDE) de un comprobante autorizado;
                           con -enviar lo envía a -para (por defecto SMTP_TEST_TO)

Configuración (variables de entorno o archivo .env; ver .env.example):
  SRI_P12_PATH, SRI_P12_PASSWORD o SRI_P12_PASSWORD_FILE, SRI_EMISOR_*
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, uso)
		os.Exit(2)
	}
	entorno.CargarEnv(".env")
	var err error
	switch orden, args := os.Args[1], os.Args[2:]; orden {
	case "certificado":
		err = cmdCertificado()
	case "validar":
		err = cmdValidar(args)
	case "firmar":
		err = cmdFirmar(args)
	case "verificar":
		err = cmdVerificar(args)
	case "factura-prueba":
		err = cmdFacturaPrueba(args)
	case "enviar":
		err = cmdEnviar(args)
	case "autorizacion":
		err = cmdAutorizacion(args)
	case "estado":
		err = cmdEstado(args)
	case "ride":
		err = cmdRide(os.Args[2:])
	case "correo":
		err = cmdCorreo(args)
	case "-h", "--help", "ayuda":
		fmt.Print(uso)
	default:
		fmt.Fprintf(os.Stderr, "orden desconocida %q\n\n%s", orden, uso)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdCertificado() error {
	c, err := entorno.Certificado()
	if err != nil {
		return err
	}
	r := c.Resumen()
	fmt.Printf("Titular:   %s\nEmisor:    %s\nSerie:     %s\nVigencia:  %s a %s\nClave RSA: %d bits\n",
		r.Titular, r.Emisor, r.Serie, r.Desde.Format("2006-01-02"), r.Hasta.Format("2006-01-02"), r.BitsRSA)
	if r.RUC != "" {
		fmt.Println("RUC:      ", r.RUC)
	}
	fmt.Printf("Cadena:    %d certificado(s) adicional(es)\n", len(c.Cadena))
	if err := c.Vigente(time.Now()); err != nil {
		return err
	}
	// Firma y verificación de prueba, sin enviar nada a ninguna parte.
	firmado, err := xades.Firmar([]byte(`<factura id="comprobante" version="1.1.0"><prueba>1</prueba></factura>`), c, nil)
	if err != nil {
		return err
	}
	if _, err := xades.Verificar(firmado, nil); err != nil {
		return err
	}
	fmt.Println("Estado:    vigente; firma de prueba generada y verificada correctamente")
	return nil
}

func leerArg(args []string) ([]byte, string, error) {
	if len(args) == 0 {
		return nil, "", errors.New("falta el archivo XML")
	}
	ruta := args[len(args)-1]
	b, err := os.ReadFile(ruta)
	return b, ruta, err
}

func cmdValidar(args []string) error {
	doc, ruta, err := leerArg(args)
	if err != nil {
		return err
	}
	tipo, version, err := esquema.Identificar(doc)
	if err != nil {
		return err
	}
	if err := esquema.Validar(doc); err != nil {
		var ev *esquema.ErrorValidacion
		if errors.As(err, &ev) {
			for _, p := range ev.Problemas {
				fmt.Println(" -", p)
			}
			return fmt.Errorf("%s no cumple el XSD de %s %s", ruta, tipo, version)
		}
		return err
	}
	fmt.Printf("%s: %s %s cumple el XSD\n", ruta, tipo, version)
	return nil
}

func algoritmo(nombre string) (xades.Algoritmo, error) {
	switch strings.ToLower(nombre) {
	case "sha256", "":
		return xades.SHA256, nil
	case "sha1":
		return xades.SHA1, nil
	}
	return 0, fmt.Errorf("algoritmo %q desconocido (sha1 o sha256)", nombre)
}

func cmdFirmar(args []string) error {
	fs := flag.NewFlagSet("firmar", flag.ExitOnError)
	salida := fs.String("salida", "", "archivo de salida (por defecto, <entrada>.firmado.xml)")
	alg := fs.String("algoritmo", "sha256", "sha256 o sha1")
	fs.Parse(args)
	doc, ruta, err := leerArg(fs.Args())
	if err != nil {
		return err
	}
	a, err := algoritmo(*alg)
	if err != nil {
		return err
	}
	c, err := entorno.Certificado()
	if err != nil {
		return err
	}
	firmado, err := xades.Firmar(doc, c, &xades.Opciones{Algoritmo: a})
	if err != nil {
		return err
	}
	if *salida == "" {
		*salida = strings.TrimSuffix(ruta, ".xml") + ".firmado.xml"
	}
	if err := os.WriteFile(*salida, firmado, 0o600); err != nil {
		return err
	}
	fmt.Printf("firmado con %s: %s\n", a, *salida)
	return nil
}

func cmdVerificar(args []string) error {
	doc, ruta, err := leerArg(args)
	if err != nil {
		return err
	}
	f, err := xades.Verificar(doc, nil)
	if err != nil {
		return err
	}
	fmt.Printf("%s: firma válida\n  firmante:  %s\n  emisor:    %s\n  fecha:     %s\n  algoritmo: %s\n  referencias: %d\n",
		ruta, f.Certificado.Subject.CommonName, f.Certificado.Issuer.CommonName, f.FechaFirma.Format(time.RFC3339), f.AlgoritmoFirma, len(f.Referencias))
	fmt.Println("  nota: no se evaluó la cadena de confianza ni la revocación del certificado")
	return nil
}

func nuevoCliente(c *certificado.Certificado, a xades.Algoritmo) (*sri.Cliente, error) {
	// Este comando solo opera contra el ambiente de pruebas.
	return sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: c, Algoritmo: a, RUC: os.Getenv("SRI_EMISOR_RUC")})
}

func cmdFacturaPrueba(args []string) error {
	fs := flag.NewFlagSet("factura-prueba", flag.ExitOnError)
	alg := fs.String("algoritmo", "sha256", "sha256 o sha1")
	secuencial := fs.Int("secuencial", 0, "número secuencial (obligatorio, 1-999999999)")
	version := fs.String("version", factura.VersionPorDefecto, "versión del esquema de factura")
	enviar := fs.Bool("enviar", false, "envía la factura al ambiente de PRUEBAS del SRI (sin esta opción solo se genera y firma)")
	dir := fs.String("dir", "evidencia-privada", "directorio donde guardar los XML y respuestas")
	fs.Parse(args)
	if *secuencial < 1 {
		return errors.New("indique -secuencial con un número que no haya usado antes en el ambiente de pruebas")
	}
	a, err := algoritmo(*alg)
	if err != nil {
		return err
	}
	c, err := entorno.Certificado()
	if err != nil {
		return err
	}
	emisor, err := entorno.Emisor(comprobante.CodFactura, *secuencial)
	if err != nil {
		return err
	}
	f := &factura.Factura{
		Version:        *version,
		InfoTributaria: emisor,
		InfoFactura: factura.InfoFactura{
			FechaEmision:                comprobante.Hoy(),
			DirEstablecimiento:          entorno.Valor("SRI_EMISOR_DIR_ESTABLECIMIENTO", os.Getenv("SRI_EMISOR_DIR_MATRIZ")),
			ObligadoContabilidad:        entorno.Valor("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO"),
			ContribuyenteEspecial:       os.Getenv("SRI_EMISOR_CONTRIBUYENTE_ESPECIAL"),
			TipoIdentificacionComprador: comprobante.IDConsumidorFinal,
			RazonSocialComprador:        comprobante.RazonSocialConsumidorFinal,
			IdentificacionComprador:     comprobante.IdentificacionConsumidorFinal,
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}},
		},
		Detalles: []factura.Detalle{
			factura.LineaIVA("PRUEBA-001", "PRUEBA DE INTEROPERABILIDAD SDK GO "+a.String(), decimal.Entero(1), decimal.MustParse("1.00"), comprobante.IVA15),
		},
	}
	f.Calcular()
	if _, err := f.GenerarClaveAcceso(""); err != nil {
		return err
	}
	cli, err := nuevoCliente(c, a)
	if err != nil {
		return err
	}
	res, err := cli.Preparar(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	base := filepath.Join(*dir, res.ClaveAcceso)
	if err := os.WriteFile(base+".firmado.xml", res.XMLFirmado, 0o600); err != nil {
		return err
	}
	fmt.Printf("clave de acceso: %s\nfirmado (%s): %s.firmado.xml\n", res.ClaveAcceso, a, base)
	fmt.Println("validación local: reglas tributarias OK, XSD OK, firma verificada")
	if !*enviar {
		fmt.Println("no se envió nada al SRI (añada -enviar para hacerlo)")
		return nil
	}
	return emitir(cli, res.XMLFirmado, base)
}

func emitir(cli *sri.Cliente, firmado []byte, base string) error {
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelar()
	res, err := cli.Reanudar(ctx, firmado, nil)
	if res != nil {
		if rec := res.Recepcion; rec != nil {
			fmt.Printf("recepción: %s\n", rec.Estado)
			for _, m := range rec.Mensajes() {
				fmt.Printf("  %s %s\n", m.Tipo, m)
			}
		}
		fmt.Printf("estado: %s\n", res.Estado)
		for _, m := range res.Mensajes {
			fmt.Printf("  %s %s\n", m.Tipo, m)
		}
		if doc := res.XMLAutorizado(); doc != nil {
			if e := os.WriteFile(base+".autorizado.xml", doc, 0o600); e != nil {
				return e
			}
			fmt.Printf("autorizado: %s.autorizado.xml\n", base)
		}
		if res.Pendiente() {
			fmt.Printf("el SRI aún no resuelve; consulte luego con: sri autorizacion %s\n", res.ClaveAcceso)
		}
	}
	return err
}

func cmdEnviar(args []string) error {
	doc, ruta, err := leerArg(args)
	if err != nil {
		return err
	}
	if _, err := xades.Verificar(doc, nil); err != nil {
		return fmt.Errorf("no se envía: %w", err)
	}
	cli, err := nuevoCliente(nil, xades.SHA256)
	if err != nil {
		return err
	}
	return emitir(cli, doc, strings.TrimSuffix(ruta, ".firmado.xml"))
}

func cmdAutorizacion(args []string) error {
	if len(args) != 1 {
		return errors.New("indique la clave de acceso")
	}
	cli, err := nuevoCliente(nil, xades.SHA256)
	if err != nil {
		return err
	}
	ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
	defer cancelar()
	res, err := cli.Autorizar(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("estado: %s\n", res.Estado)
	if t, ok := res.FechaAutorizacion(); ok {
		fmt.Println("fecha: ", t.Format(time.RFC3339))
	}
	for _, m := range res.Mensajes {
		fmt.Printf("  %s %s\n", m.Tipo, m)
	}
	return nil
}

func cmdEstado(args []string) error {
	if len(args) != 1 {
		return errors.New("indique la clave de acceso")
	}
	cli, err := nuevoCliente(nil, xades.SHA256)
	if err != nil {
		return err
	}
	ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
	defer cancelar()
	e, err := cli.Servicios().ConsultarEstado(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("estadoConsulta: %q\nestadoAutorizacion: %q\ntipoComprobante: %q\nrucEmisor: %q\nfechaAutorizacion: %q\n",
		e.EstadoConsulta, e.EstadoAutorizacion, e.TipoComprobante, e.RucEmisor, e.FechaAutorizacion)
	for _, m := range e.Mensajes {
		fmt.Printf("  %s %s\n", m.Tipo, m)
	}
	return nil
}

// cmdCorreo compone, y con -enviar envía, el correo de entrega de un
// comprobante autorizado.
func cmdRide(args []string) error {
	fs := flag.NewFlagSet("ride", flag.ExitOnError)
	salida := fs.String("salida", "", "archivo PDF de salida (por defecto, el de entrada con extensión .pdf)")
	rutaLogo := fs.String("logo", os.Getenv("SRI_EMISOR_LOGO"), "logotipo PNG o JPEG (por defecto SRI_EMISOR_LOGO)")
	fs.Parse(args)
	doc, ruta, err := leerArg(fs.Args())
	if err != nil {
		return err
	}
	var logo []byte
	if *rutaLogo != "" {
		if logo, err = os.ReadFile(*rutaLogo); err != nil {
			return err
		}
	}
	pdf, err := ride.Generar(doc, &ride.Opciones{Logo: logo})
	if err != nil {
		return err
	}
	if *salida == "" {
		*salida = strings.TrimSuffix(ruta, ".xml") + ".pdf"
	}
	if err := os.WriteFile(*salida, pdf, 0o600); err != nil {
		return err
	}
	fmt.Printf("RIDE guardado en %s (%d bytes)\n", *salida, len(pdf))
	return nil
}

func cmdCorreo(args []string) error {
	fs := flag.NewFlagSet("correo", flag.ExitOnError)
	para := fs.String("para", os.Getenv("SMTP_TEST_TO"), "destinatario (por defecto SMTP_TEST_TO)")
	enviar := fs.Bool("enviar", false, "envía el correo por el servidor SMTP configurado (sin esta opción solo se compone)")
	salida := fs.String("salida", "", "guarda el mensaje compuesto (.eml) en este archivo")
	sinRide := fs.Bool("sin-ride", false, "no adjunta el RIDE en PDF")
	rutaLogo := fs.String("logo", os.Getenv("SRI_EMISOR_LOGO"), "logotipo PNG o JPEG para el RIDE (por defecto SRI_EMISOR_LOGO)")
	fs.Parse(args)
	doc, _, err := leerArg(fs.Args())
	if err != nil {
		return err
	}
	var aut struct {
		Estado      string `xml:"estado"`
		Comprobante string `xml:"comprobante"`
	}
	if err := xml.Unmarshal(doc, &aut); err != nil || aut.Estado != "AUTORIZADO" || aut.Comprobante == "" {
		return errors.New("el archivo no es un comprobante AUTORIZADO (use el .autorizado.xml)")
	}
	res, err := ride.Resumir(doc)
	if err != nil {
		return err
	}
	datos := correo.Comprobante{Tipo: res.Tipo, Numero: res.Numero, ClaveAcceso: res.ClaveAcceso, Emisor: res.Emisor, Receptor: res.Receptor, XML: doc}
	if !*sinRide {
		var logo []byte
		if *rutaLogo != "" {
			if logo, err = os.ReadFile(*rutaLogo); err != nil {
				return err
			}
		}
		if datos.RIDE, err = ride.Generar(doc, &ride.Opciones{Logo: logo}); err != nil {
			return err
		}
	}
	puerto, _ := strconv.Atoi(entorno.Valor("SMTP_PORT", "587"))
	cli, err := correo.Nuevo(correo.Config{
		Host: os.Getenv("SMTP_HOST"), Puerto: puerto, Seguridad: correo.Seguridad(strings.ToLower(os.Getenv("SMTP_TLS"))),
		Usuario: os.Getenv("SMTP_USER"), Contrasena: os.Getenv("SMTP_PASSWORD"),
		Remitente: os.Getenv("SMTP_FROM"), NombreRemitente: datos.Emisor,
	})
	if err != nil {
		return err
	}
	if *para == "" {
		return errors.New("indique -para o defina SMTP_TEST_TO")
	}
	m := correo.DeComprobante(datos)
	m.Para = []string{*para}
	eml, err := cli.Componer(m)
	if err != nil {
		return err
	}
	fmt.Printf("asunto: %s\nadjuntos: %d\ntamaño: %d bytes\n", m.Asunto, len(m.Adjuntos), len(eml))
	if *salida != "" {
		if err := os.WriteFile(*salida, eml, 0o600); err != nil {
			return err
		}
		fmt.Println("mensaje guardado en", *salida)
	}
	if !*enviar {
		fmt.Println("no se envió nada (añada -enviar para hacerlo)")
		return nil
	}
	ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
	defer cancelar()
	if err := cli.Enviar(ctx, m); err != nil {
		return err
	}
	fmt.Println("correo aceptado por el servidor SMTP")
	return nil
}

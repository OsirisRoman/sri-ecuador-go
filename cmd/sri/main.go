// Comando sri: utilidades de línea de comandos del SDK. Sirve para
// comprobar un certificado, validar, firmar y verificar comprobantes, y para
// ejecutar pruebas reales contra el ambiente de certificación del SRI.
//
// La configuración privada se toma de variables de entorno o de un archivo
// .env (ver .env.example). La contraseña del certificado nunca se pasa por
// argumentos de línea de comandos.
package main

import (
	"bufio"
	"context"
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

Configuración (variables de entorno o archivo .env; ver .env.example):
  SRI_P12_PATH, SRI_P12_PASSWORD o SRI_P12_PASSWORD_FILE, SRI_EMISOR_*
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, uso)
		os.Exit(2)
	}
	cargarEnv(".env")
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

// cargarEnv lee pares CLAVE=valor de un archivo sin sobrescribir variables
// ya definidas en el entorno.
func cargarEnv(ruta string) {
	f, err := os.Open(ruta)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		linea := strings.TrimSpace(sc.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		k, v, ok := strings.Cut(linea, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if _, existe := os.LookupEnv(k); !existe {
			os.Setenv(k, v)
		}
	}
}

func cargarCertificado() (*certificado.Certificado, error) {
	ruta := os.Getenv("SRI_P12_PATH")
	if ruta == "" {
		return nil, errors.New("defina SRI_P12_PATH con la ruta del archivo .p12")
	}
	clave := os.Getenv("SRI_P12_PASSWORD")
	if f := os.Getenv("SRI_P12_PASSWORD_FILE"); clave == "" && f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer SRI_P12_PASSWORD_FILE: %w", err)
		}
		clave = strings.TrimRight(string(b), "\r\n")
	}
	if clave == "" {
		return nil, errors.New("defina SRI_P12_PASSWORD o SRI_P12_PASSWORD_FILE")
	}
	if info, err := os.Stat(ruta); err == nil && info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "aviso: %s es legible por otros usuarios; se recomienda chmod 600\n", ruta)
	}
	return certificado.CargarArchivo(ruta, clave)
}

func cmdCertificado() error {
	c, err := cargarCertificado()
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
	case "sha1", "":
		return xades.SHA1, nil
	case "sha256":
		return xades.SHA256, nil
	}
	return 0, fmt.Errorf("algoritmo %q desconocido (sha1 o sha256)", nombre)
}

func cmdFirmar(args []string) error {
	fs := flag.NewFlagSet("firmar", flag.ExitOnError)
	salida := fs.String("salida", "", "archivo de salida (por defecto, <entrada>.firmado.xml)")
	alg := fs.String("algoritmo", "sha1", "sha1 o sha256")
	fs.Parse(args)
	doc, ruta, err := leerArg(fs.Args())
	if err != nil {
		return err
	}
	a, err := algoritmo(*alg)
	if err != nil {
		return err
	}
	c, err := cargarCertificado()
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

func env(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}

func nuevoCliente(c *certificado.Certificado, a xades.Algoritmo) (*sri.Cliente, error) {
	// Este comando solo opera contra el ambiente de pruebas.
	return sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: c, Algoritmo: a, RUC: os.Getenv("SRI_EMISOR_RUC")})
}

func cmdFacturaPrueba(args []string) error {
	fs := flag.NewFlagSet("factura-prueba", flag.ExitOnError)
	alg := fs.String("algoritmo", "sha1", "sha1 o sha256")
	secuencial := fs.Int("secuencial", 0, "número secuencial (obligatorio, 1-999999999)")
	version := fs.String("version", factura.VersionPorDefecto, "versión del esquema de factura")
	enviar := fs.Bool("enviar", false, "envía la factura al ambiente de PRUEBAS del SRI (sin esta opción solo se genera y firma)")
	dir := fs.String("dir", "evidencia-privada", "directorio donde guardar los XML y respuestas")
	fs.Parse(args)
	if *secuencial < 1 || *secuencial > 999999999 {
		return errors.New("indique -secuencial con un número que no haya usado antes en el ambiente de pruebas")
	}
	a, err := algoritmo(*alg)
	if err != nil {
		return err
	}
	for _, v := range []string{"SRI_EMISOR_RUC", "SRI_EMISOR_RAZON_SOCIAL", "SRI_EMISOR_DIR_MATRIZ"} {
		if os.Getenv(v) == "" {
			return fmt.Errorf("falta la variable %s (ver .env.example)", v)
		}
	}
	c, err := cargarCertificado()
	if err != nil {
		return err
	}
	f := &factura.Factura{
		Version: *version,
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, TipoEmision: comprobante.EmisionNormal,
			RazonSocial:        os.Getenv("SRI_EMISOR_RAZON_SOCIAL"),
			NombreComercial:    os.Getenv("SRI_EMISOR_NOMBRE_COMERCIAL"),
			RUC:                os.Getenv("SRI_EMISOR_RUC"),
			CodDoc:             comprobante.CodFactura,
			Estab:              env("SRI_EMISOR_ESTAB", "001"),
			PtoEmi:             env("SRI_EMISOR_PTO_EMI", "001"),
			Secuencial:         fmt.Sprintf("%09d", *secuencial),
			DirMatriz:          os.Getenv("SRI_EMISOR_DIR_MATRIZ"),
			AgenteRetencion:    os.Getenv("SRI_EMISOR_AGENTE_RETENCION"),
			ContribuyenteRimpe: os.Getenv("SRI_EMISOR_CONTRIBUYENTE_RIMPE"),
		},
		InfoFactura: factura.InfoFactura{
			FechaEmision:                comprobante.Hoy(),
			DirEstablecimiento:          env("SRI_EMISOR_DIR_ESTABLECIMIENTO", os.Getenv("SRI_EMISOR_DIR_MATRIZ")),
			ObligadoContabilidad:        env("SRI_EMISOR_OBLIGADO_CONTABILIDAD", "NO"),
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
	cli, err := nuevoCliente(nil, xades.SHA1)
	if err != nil {
		return err
	}
	return emitir(cli, doc, strings.TrimSuffix(ruta, ".firmado.xml"))
}

func cmdAutorizacion(args []string) error {
	if len(args) != 1 {
		return errors.New("indique la clave de acceso")
	}
	cli, err := nuevoCliente(nil, xades.SHA1)
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

// Package correo envía por SMTP los comprobantes electrónicos autorizados
// a sus receptores, como exige la normativa (el emisor debe entregar el
// comprobante al adquirente; ficha técnica v2.34, sección 5.12).
//
// Es un paquete opcional: el resto del SDK no lo usa ni depende de un
// servidor de correo. Solo emplea la biblioteca estándar.
//
//	c, _ := correo.Nuevo(correo.Config{Host: "smtp.ejemplo.com", Puerto: 587,
//	    Usuario: u, Contrasena: p, Remitente: "facturas@ejemplo.com"})
//	m := correo.DeComprobante(correo.Comprobante{ … XML: res.XMLAutorizado() })
//	m.Para = []string{"cliente@ejemplo.com"}
//	err := c.Enviar(ctx, m)
package correo

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Seguridad es el modo de cifrado de la conexión SMTP.
type Seguridad string

const (
	// STARTTLS abre la conexión en claro y la eleva a TLS antes de
	// autenticarse (habitual en el puerto 587). Si el servidor no ofrece
	// STARTTLS el envío falla: nunca se degrada a texto claro.
	STARTTLS Seguridad = "starttls"
	// TLS establece TLS desde el primer byte (habitual en el puerto 465).
	TLS Seguridad = "tls"
	// SinCifrado no cifra. Solo para servidores de prueba locales.
	SinCifrado Seguridad = "ninguna"
)

// Errores.
var (
	// ErrConfig indica una configuración incompleta o inválida.
	ErrConfig = errors.New("correo: configuración inválida")
	// ErrMensaje indica un mensaje que no se puede enviar tal como está.
	ErrMensaje = errors.New("correo: mensaje inválido")
	// ErrSinSTARTTLS indica que el servidor no ofrece STARTTLS.
	ErrSinSTARTTLS = errors.New("correo: el servidor no ofrece STARTTLS; no se envía sin cifrar")
)

// Config configura el acceso al servidor SMTP.
type Config struct {
	Host   string
	Puerto int
	// Seguridad; vacío equivale a TLS si el puerto es 465 y a STARTTLS en
	// los demás casos.
	Seguridad Seguridad
	// Usuario y Contrasena; si Usuario está vacío no se autentica.
	Usuario    string
	Contrasena string
	// Remitente es la dirección del emisor (cabecera From y sobre SMTP).
	Remitente string
	// NombreRemitente es el nombre visible, opcional.
	NombreRemitente string
	// TLS permite ajustar la verificación (por ejemplo, una CA privada).
	// Por defecto se verifica el certificado del servidor contra Host.
	TLS *tls.Config
	// Plazo máximo de cada envío si el contexto no fija uno. Por defecto,
	// 60 s.
	Plazo time.Duration
}

// String omite la contraseña para que no acabe en un log.
func (c Config) String() string {
	return fmt.Sprintf("correo.Config{%s:%d %s usuario=%q remitente=%q}", c.Host, c.Puerto, c.Seguridad, c.Usuario, c.Remitente)
}

// GoString evita lo mismo con %#v.
func (c Config) GoString() string { return c.String() }

// Adjunto es un archivo adjunto.
type Adjunto struct {
	Nombre string
	// Tipo MIME; vacío equivale a application/octet-stream.
	Tipo  string
	Datos []byte
}

// Mensaje es un correo.
type Mensaje struct {
	Para, CC, CCO []string
	Asunto        string
	// Texto es el cuerpo en texto plano. HTML, si se indica, se envía como
	// alternativa.
	Texto    string
	HTML     string
	Adjuntos []Adjunto
}

// Cliente envía mensajes. Es seguro para uso concurrente: cada envío abre
// su propia conexión.
type Cliente struct {
	cfg Config
	de  mail.Address
}

// Nuevo valida la configuración y crea un Cliente. No se conecta.
func Nuevo(cfg Config) (*Cliente, error) {
	if cfg.Host == "" || strings.ContainsAny(cfg.Host, " \r\n") {
		return nil, fmt.Errorf("%w: falta Host", ErrConfig)
	}
	if cfg.Puerto <= 0 || cfg.Puerto > 65535 {
		return nil, fmt.Errorf("%w: Puerto %d", ErrConfig, cfg.Puerto)
	}
	switch cfg.Seguridad {
	case "":
		cfg.Seguridad = STARTTLS
		if cfg.Puerto == 465 {
			cfg.Seguridad = TLS
		}
	case STARTTLS, TLS, SinCifrado:
	default:
		return nil, fmt.Errorf("%w: Seguridad %q (use starttls, tls o ninguna)", ErrConfig, cfg.Seguridad)
	}
	de, err := direccion(cfg.Remitente)
	if err != nil {
		return nil, fmt.Errorf("%w: Remitente: %v", ErrConfig, err)
	}
	if strings.ContainsAny(cfg.NombreRemitente, "\r\n") {
		return nil, fmt.Errorf("%w: NombreRemitente con salto de línea", ErrConfig)
	}
	if cfg.Plazo <= 0 {
		cfg.Plazo = 60 * time.Second
	}
	return &Cliente{cfg: cfg, de: mail.Address{Name: cfg.NombreRemitente, Address: de}}, nil
}

// direccion valida una dirección y devuelve solo la parte usuario@dominio.
// Rechazar saltos de línea impide la inyección de cabeceras.
func direccion(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n") {
		return "", errors.New("dirección con salto de línea")
	}
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("dirección %q inválida", s)
	}
	return a.Address, nil
}

// Enviar entrega el mensaje. Devuelve error si el servidor rechaza la
// conexión, la autenticación, al remitente o a cualquiera de los
// destinatarios; en ese último caso no se entrega a ninguno.
func (c *Cliente) Enviar(ctx context.Context, m Mensaje) error {
	var destinos []string
	for _, lista := range [][]string{m.Para, m.CC, m.CCO} {
		for _, d := range lista {
			a, err := direccion(d)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrMensaje, err)
			}
			destinos = append(destinos, a)
		}
	}
	if len(destinos) == 0 {
		return fmt.Errorf("%w: sin destinatarios", ErrMensaje)
	}
	cuerpo, err := c.Componer(m)
	if err != nil {
		return err
	}

	if _, tiene := ctx.Deadline(); !tiene {
		var cancelar context.CancelFunc
		ctx, cancelar = context.WithTimeout(ctx, c.cfg.Plazo)
		defer cancelar()
	}
	dir := net.JoinHostPort(c.cfg.Host, strconv.Itoa(c.cfg.Puerto))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", dir)
	if err != nil {
		return fmt.Errorf("correo: no se pudo conectar a %s: %w", dir, err)
	}
	defer conn.Close()
	if limite, ok := ctx.Deadline(); ok {
		conn.SetDeadline(limite)
	}
	// Si el contexto se cancela, cerrar la conexión desbloquea la E/S.
	fin := make(chan struct{})
	defer close(fin)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-fin:
		}
	}()

	tlsCfg := &tls.Config{ServerName: c.cfg.Host, MinVersion: tls.VersionTLS12}
	if c.cfg.TLS != nil {
		tlsCfg = c.cfg.TLS.Clone()
		if tlsCfg.ServerName == "" {
			tlsCfg.ServerName = c.cfg.Host
		}
	}
	if c.cfg.Seguridad == TLS {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("correo: TLS con %s: %w", dir, err)
		}
		conn = tc
	}
	cl, err := smtp.NewClient(conn, c.cfg.Host)
	if err != nil {
		return fmt.Errorf("correo: saludo SMTP: %w", err)
	}
	defer cl.Close()
	if c.cfg.Seguridad == STARTTLS {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return ErrSinSTARTTLS
		}
		if err := cl.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("correo: STARTTLS: %w", err)
		}
	}
	if c.cfg.Usuario != "" {
		if c.cfg.Seguridad == SinCifrado && !esLocal(c.cfg.Host) {
			return fmt.Errorf("%w: no se envían credenciales sin cifrar a un servidor remoto", ErrConfig)
		}
		if err := cl.Auth(smtp.PlainAuth("", c.cfg.Usuario, c.cfg.Contrasena, c.cfg.Host)); err != nil {
			return fmt.Errorf("correo: autenticación rechazada: %w", err)
		}
	}
	if err := cl.Mail(c.de.Address); err != nil {
		return fmt.Errorf("correo: remitente rechazado: %w", err)
	}
	for _, d := range destinos {
		if err := cl.Rcpt(d); err != nil {
			return fmt.Errorf("correo: destinatario %s rechazado: %w", d, err)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("correo: %w", err)
	}
	if _, err := w.Write(cuerpo); err != nil {
		return fmt.Errorf("correo: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("correo: el servidor no aceptó el mensaje: %w", err)
	}
	return cl.Quit()
}

func esLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Componer construye el mensaje RFC 5322 / MIME completo. Es útil para
// inspeccionarlo o entregarlo por otro medio.
func (c *Cliente) Componer(m Mensaje) ([]byte, error) {
	if strings.ContainsAny(m.Asunto, "\r\n") {
		return nil, fmt.Errorf("%w: el asunto contiene saltos de línea", ErrMensaje)
	}
	if m.Texto == "" && m.HTML == "" {
		return nil, fmt.Errorf("%w: sin cuerpo", ErrMensaje)
	}
	lista := func(ds []string) (string, error) {
		var out []string
		for _, d := range ds {
			a, err := direccion(d)
			if err != nil {
				return "", fmt.Errorf("%w: %v", ErrMensaje, err)
			}
			out = append(out, "<"+a+">")
		}
		return strings.Join(out, ", "), nil
	}
	para, err := lista(m.Para)
	if err != nil {
		return nil, err
	}
	cc, err := lista(m.CC)
	if err != nil {
		return nil, err
	}
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(id[:])
	dominio := c.de.Address[strings.LastIndex(c.de.Address, "@")+1:]

	var b bytes.Buffer
	cab := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	cab("From", c.de.String())
	if para != "" {
		cab("To", para)
	}
	if cc != "" {
		cab("Cc", cc)
	}
	cab("Subject", mime.QEncoding.Encode("utf-8", m.Asunto))
	cab("Date", time.Now().Format(time.RFC1123Z))
	cab("Message-ID", "<"+token+"@"+dominio+">")
	cab("MIME-Version", "1.0")

	// Cuerpo: texto solo, o multipart/alternative con texto y HTML.
	var cuerpo bytes.Buffer
	var tipoCuerpo string
	parteTexto := func(w *bytes.Buffer, subtipo, contenido string) {
		w.WriteString("Content-Type: text/" + subtipo + "; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		qp := quotedprintable.NewWriter(w)
		qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(contenido, "\r\n", "\n"), "\n", "\r\n")))
		qp.Close()
		w.WriteString("\r\n")
	}
	switch {
	case m.Texto != "" && m.HTML != "":
		lim := "alt-" + token
		tipoCuerpo = `multipart/alternative; boundary="` + lim + `"`
		cuerpo.WriteString("--" + lim + "\r\n")
		parteTexto(&cuerpo, "plain", m.Texto)
		cuerpo.WriteString("--" + lim + "\r\n")
		parteTexto(&cuerpo, "html", m.HTML)
		cuerpo.WriteString("--" + lim + "--\r\n")
	case m.HTML != "":
		parteTexto(&cuerpo, "html", m.HTML)
	default:
		parteTexto(&cuerpo, "plain", m.Texto)
	}

	if len(m.Adjuntos) == 0 {
		if tipoCuerpo != "" {
			cab("Content-Type", tipoCuerpo)
			b.WriteString("\r\n")
		}
		// Sin multipart, las cabeceras de la parte son las del mensaje.
		b.Write(cuerpo.Bytes())
		return b.Bytes(), nil
	}
	lim := "mix-" + token
	cab("Content-Type", `multipart/mixed; boundary="`+lim+`"`)
	b.WriteString("\r\n--" + lim + "\r\n")
	if tipoCuerpo != "" {
		b.WriteString("Content-Type: " + tipoCuerpo + "\r\n\r\n")
	}
	b.Write(cuerpo.Bytes())
	for _, a := range m.Adjuntos {
		if a.Nombre == "" || strings.ContainsAny(a.Nombre, "\r\n/\\\"") {
			return nil, fmt.Errorf("%w: nombre de adjunto %q", ErrMensaje, a.Nombre)
		}
		tipo := a.Tipo
		if tipo == "" {
			tipo = "application/octet-stream"
		}
		if strings.ContainsAny(tipo, "\r\n") {
			return nil, fmt.Errorf("%w: tipo de adjunto inválido", ErrMensaje)
		}
		nombre := mime.QEncoding.Encode("utf-8", a.Nombre)
		b.WriteString("--" + lim + "\r\n")
		b.WriteString("Content-Type: " + tipo + `; name="` + nombre + "\"\r\n")
		b.WriteString(`Content-Disposition: attachment; filename="` + nombre + "\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString(a.Datos)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	b.WriteString("--" + lim + "--\r\n")
	return b.Bytes(), nil
}

// Comprobante reúne los datos con los que DeComprobante redacta el correo
// de entrega de un comprobante autorizado.
type Comprobante struct {
	// Tipo legible: "Factura", "Nota de crédito", …
	Tipo string
	// Numero con formato 001-001-000000001.
	Numero string
	// ClaveAcceso, que es también el número de autorización.
	ClaveAcceso string
	Emisor      string
	Receptor    string
	// XML es el comprobante autorizado (sri.Resultado.XMLAutorizado).
	XML []byte
	// RIDE es la representación impresa en PDF, si se dispone de ella.
	RIDE []byte
}

// DeComprobante redacta el mensaje de entrega de un comprobante autorizado,
// con el XML (y el RIDE, si se da) adjuntos. Quedan por asignar los
// destinatarios.
func DeComprobante(c Comprobante) Mensaje {
	m := Mensaje{
		Asunto: fmt.Sprintf("%s electrónica %s de %s", c.Tipo, c.Numero, c.Emisor),
		Texto: fmt.Sprintf("Estimado(a) %s:\n\n%s le ha emitido el siguiente comprobante electrónico, autorizado por el Servicio de Rentas Internas.\n\n"+
			"Tipo: %s\nNúmero: %s\nClave de acceso / autorización: %s\n\n"+
			"Se adjunta el comprobante en formato XML, que es el documento con validez tributaria. "+
			"Puede verificarlo en el portal del SRI (www.sri.gob.ec) con la clave de acceso.\n",
			c.Receptor, c.Emisor, c.Tipo, c.Numero, c.ClaveAcceso),
	}
	if len(c.XML) > 0 {
		m.Adjuntos = append(m.Adjuntos, Adjunto{Nombre: c.ClaveAcceso + ".xml", Tipo: "application/xml", Datos: c.XML})
	}
	if len(c.RIDE) > 0 {
		m.Adjuntos = append(m.Adjuntos, Adjunto{Nombre: c.ClaveAcceso + ".pdf", Tipo: "application/pdf", Datos: c.RIDE})
	}
	return m
}

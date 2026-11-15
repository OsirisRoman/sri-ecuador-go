package correo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"testing"
)

// servidorSMTP es un servidor mínimo en claro que acepta un mensaje.
type servidorSMTP struct {
	ln       net.Listener
	de       string
	para     []string
	datos    string
	starttls bool
}

func nuevoServidor(t *testing.T) *servidorSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &servidorSMTP{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		decir := func(l string) { fmt.Fprint(conn, l+"\r\n") }
		decir("220 prueba ESMTP")
		for {
			linea, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(linea))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				decir("250-prueba")
				decir("250 8BITMIME")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				s.de = strings.TrimSpace(linea[10:])
				decir("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				s.para = append(s.para, strings.TrimSpace(linea[8:]))
				decir("250 ok")
			case cmd == "DATA":
				decir("354 adelante")
				var sb strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					sb.WriteString(l)
				}
				s.datos = sb.String()
				decir("250 aceptado")
			case cmd == "QUIT":
				decir("221 adios")
				return
			default:
				decir("502 no implementado")
			}
		}
	}()
	return s
}

func (s *servidorSMTP) puerto() int { return s.ln.Addr().(*net.TCPAddr).Port }

func TestEnviarComprobante(t *testing.T) {
	srv := nuevoServidor(t)
	c, err := Nuevo(Config{Host: "127.0.0.1", Puerto: srv.puerto(), Seguridad: SinCifrado, Remitente: "facturas@emisor.example", NombreRemitente: "Ñandú & Cía."})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc := bytes.Repeat([]byte("<autorización>ñ</autorización>\n"), 20)
	m := DeComprobante(Comprobante{Tipo: "Factura", Numero: "001-001-000000123", ClaveAcceso: strings.Repeat("1", 49),
		Emisor: "EMISOR DE PRUEBA S.A.", Receptor: "José Peña", XML: xmlDoc, RIDE: []byte("%PDF-1.4 falso")})
	m.Para = []string{"Cliente <cliente@receptor.example>"}
	m.CCO = []string{"archivo@emisor.example"}
	m.HTML = "<p>Hola <b>José</b></p>"
	if err := c.Enviar(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(srv.de, "<facturas@emisor.example>") || len(srv.para) != 2 {
		t.Errorf("sobre: de=%s para=%v", srv.de, srv.para)
	}
	msg, err := mail.ReadMessage(strings.NewReader(srv.datos))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	if asunto, _ := dec.DecodeHeader(msg.Header.Get("Subject")); asunto != "Factura electrónica 001-001-000000123 de EMISOR DE PRUEBA S.A." {
		t.Errorf("asunto = %q", asunto)
	}
	if de, _ := dec.DecodeHeader(msg.Header.Get("From")); !strings.Contains(de, "Ñandú & Cía.") {
		t.Errorf("From = %q", de)
	}
	if strings.Contains(srv.datos, "archivo@emisor.example") {
		t.Error("la copia oculta no debe aparecer en las cabeceras")
	}
	tipo, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || tipo != "multipart/mixed" {
		t.Fatalf("Content-Type = %s %v", tipo, err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var nombres []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		datos, _ := io.ReadAll(p)
		if n := p.FileName(); n != "" {
			nombres = append(nombres, n)
			if strings.HasSuffix(n, ".xml") {
				// multipart decodifica base64 de forma transparente solo para
				// quoted-printable; aquí se decodifica a mano.
				limpio := strings.NewReplacer("\r", "", "\n", "").Replace(string(datos))
				if got := decodificar(t, limpio); !bytes.Equal(got, xmlDoc) {
					t.Error("el XML adjunto no coincide con el original")
				}
				for _, l := range strings.Split(strings.TrimSpace(string(datos)), "\r\n") {
					if len(l) > 76 {
						t.Errorf("línea base64 de %d caracteres", len(l))
					}
				}
			}
		}
	}
	if len(nombres) != 2 || !strings.HasSuffix(nombres[0], ".xml") || !strings.HasSuffix(nombres[1], ".pdf") {
		t.Errorf("adjuntos = %v", nombres)
	}
}

func decodificar(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRechazos(t *testing.T) {
	ok := Config{Host: "smtp.example.com", Puerto: 587, Remitente: "a@example.com"}
	for nombre, mut := range map[string]func(*Config){
		"sin host":            func(c *Config) { c.Host = "" },
		"puerto":              func(c *Config) { c.Puerto = 0 },
		"remitente":           func(c *Config) { c.Remitente = "no-es-correo" },
		"remitente inyectado": func(c *Config) { c.Remitente = "a@example.com\r\nBcc: x@example.com" },
		"seguridad":           func(c *Config) { c.Seguridad = "ssl3" },
	} {
		cfg := ok
		mut(&cfg)
		if _, err := Nuevo(cfg); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	c, _ := Nuevo(ok)
	for nombre, m := range map[string]Mensaje{
		"sin destinatarios": {Asunto: "x", Texto: "y"},
		"asunto inyectado":  {Para: []string{"b@example.com"}, Asunto: "x\r\nBcc: c@example.com", Texto: "y"},
		"destino inyectado": {Para: []string{"b@example.com\r\nRCPT TO:<c@example.com>"}, Asunto: "x", Texto: "y"},
		"sin cuerpo":        {Para: []string{"b@example.com"}, Asunto: "x"},
		"adjunto con ruta":  {Para: []string{"b@example.com"}, Asunto: "x", Texto: "y", Adjuntos: []Adjunto{{Nombre: "../a.xml"}}},
	} {
		if err := c.Enviar(context.Background(), m); !errors.Is(err, ErrMensaje) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	// Valores por defecto y ocultación de la contraseña.
	if c.cfg.Seguridad != STARTTLS {
		t.Errorf("seguridad por defecto = %s", c.cfg.Seguridad)
	}
	c465, _ := Nuevo(Config{Host: "smtp.example.com", Puerto: 465, Remitente: "a@example.com", Contrasena: "s3creta"})
	if c465.cfg.Seguridad != TLS || strings.Contains(fmt.Sprintf("%v %+v %#v", c465.cfg, c465.cfg, c465.cfg), "s3creta") {
		t.Error("puerto 465 debe usar TLS y la contraseña no debe imprimirse")
	}
}

// Con STARTTLS exigido, un servidor que no lo ofrece no recibe nada.
func TestNoDegradaATextoClaro(t *testing.T) {
	srv := nuevoServidor(t)
	c, _ := Nuevo(Config{Host: "127.0.0.1", Puerto: srv.puerto(), Seguridad: STARTTLS, Usuario: "u", Contrasena: "p", Remitente: "a@example.com"})
	err := c.Enviar(context.Background(), Mensaje{Para: []string{"b@example.com"}, Asunto: "x", Texto: "y"})
	if !errors.Is(err, ErrSinSTARTTLS) || srv.datos != "" || srv.de != "" {
		t.Errorf("err=%v datos=%q", err, srv.datos)
	}
}

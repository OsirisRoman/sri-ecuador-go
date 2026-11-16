// Package certificado carga certificados de firma electrónica en formato
// PKCS#12 (.p12 / .pfx), el archivo de intercambio que exige la ficha
// técnica del SRI (sección 6.8).
//
// Los .p12 emitidos por las entidades de certificación ecuatorianas no son
// uniformes: algunos traen la cadena completa de la CA, otros incluyen más de
// una clave privada (por ejemplo una de firma y otra de cifrado). Cargar
// elige, entre todos los pares clave-certificado del archivo, el que sirve
// para firmar.
//
// Seguridad: la clave privada nunca sale del proceso; este paquete no la
// escribe en disco, no la registra en logs y no implementa su exportación.
package certificado

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// Errores que permiten a la aplicación reaccionar sin analizar textos.
var (
	// ErrContrasena indica que la contraseña del PKCS#12 es incorrecta.
	ErrContrasena = errors.New("certificado: contraseña incorrecta")
	// ErrFormato indica que el archivo no es un PKCS#12 legible.
	ErrFormato = errors.New("certificado: el archivo no es un PKCS#12 válido")
	// ErrSinParDeFirma indica que el archivo no contiene ningún certificado
	// con su clave privada RSA correspondiente.
	ErrSinParDeFirma = errors.New("certificado: no hay un certificado con clave privada RSA utilizable para firmar")
	// ErrVencido indica que el certificado ya no está vigente.
	ErrVencido = errors.New("certificado: vencido")
	// ErrAunNoVigente indica que el certificado todavía no entra en vigencia.
	ErrAunNoVigente = errors.New("certificado: aún no vigente")
	// ErrClaveDebil indica una clave RSA menor a 2048 bits (sección 6.8).
	ErrClaveDebil = errors.New("certificado: la clave RSA tiene menos de 2048 bits")
)

// Certificado es un certificado de firma junto con su clave privada.
type Certificado struct {
	// X509 es el certificado del firmante.
	X509 *x509.Certificate
	// Cadena son los demás certificados del archivo (CA intermedias y
	// raíz), sin orden garantizado.
	Cadena []*x509.Certificate

	clave *rsa.PrivateKey
}

// Cargar decodifica un PKCS#12 en memoria.
//
// No valida la vigencia: use Vigente para ello, de modo que la aplicación
// decida qué hacer con un certificado vencido (por ejemplo, verificar firmas
// antiguas).
func Cargar(p12 []byte, contrasena string) (*Certificado, error) {
	// Camino común: una sola clave privada.
	if k, cert, cadena, err := pkcs12.DecodeChain(p12, contrasena); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok && coincide(cert, rk) {
			return &Certificado{X509: cert, Cadena: cadena, clave: rk}, nil
		}
	} else if errors.Is(err, pkcs12.ErrIncorrectPassword) {
		return nil, ErrContrasena
	}

	// Camino general: varias claves o bolsas en orden inusual.
	bloques, err := pkcs12.ToPEM(p12, contrasena)
	if err != nil {
		if errors.Is(err, pkcs12.ErrIncorrectPassword) {
			return nil, ErrContrasena
		}
		return nil, fmt.Errorf("%w: %v", ErrFormato, err)
	}
	var claves []*rsa.PrivateKey
	var certs []*x509.Certificate
	for _, b := range bloques {
		switch b.Type {
		case "CERTIFICATE":
			if c, err := x509.ParseCertificate(b.Bytes); err == nil {
				certs = append(certs, c)
			}
		case "PRIVATE KEY":
			if k, err := x509.ParsePKCS8PrivateKey(b.Bytes); err == nil {
				if rk, ok := k.(*rsa.PrivateKey); ok {
					claves = append(claves, rk)
				}
			} else if rk, err := x509.ParsePKCS1PrivateKey(b.Bytes); err == nil {
				claves = append(claves, rk)
			}
		}
	}
	type par struct {
		cert  *x509.Certificate
		clave *rsa.PrivateKey
	}
	var pares []par
	for _, c := range certs {
		for _, k := range claves {
			if coincide(c, k) {
				pares = append(pares, par{c, k})
				break
			}
		}
	}
	if len(pares) == 0 {
		return nil, ErrSinParDeFirma
	}
	// Preferencia: certificados de entidad final con uso de firma digital
	// o no repudio; entre ellos, el de vencimiento más lejano.
	puntaje := func(c *x509.Certificate) int {
		p := 0
		if !c.IsCA {
			p += 4
		}
		if c.KeyUsage == 0 || c.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) != 0 {
			p += 2
		}
		return p
	}
	sort.SliceStable(pares, func(i, j int) bool {
		pi, pj := puntaje(pares[i].cert), puntaje(pares[j].cert)
		if pi != pj {
			return pi > pj
		}
		return pares[i].cert.NotAfter.After(pares[j].cert.NotAfter)
	})
	elegido := pares[0]
	out := &Certificado{X509: elegido.cert, clave: elegido.clave}
	for _, c := range certs {
		if c != elegido.cert {
			out.Cadena = append(out.Cadena, c)
		}
	}
	return out, nil
}

// CargarArchivo lee y decodifica un archivo .p12.
func CargarArchivo(ruta, contrasena string) (*Certificado, error) {
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return nil, fmt.Errorf("certificado: %w", err)
	}
	return Cargar(datos, contrasena)
}

// DesdeClave construye un Certificado a partir de un certificado X.509 y su
// clave privada ya cargados (por ejemplo, desde un almacén propio o en
// pruebas).
func DesdeClave(cert *x509.Certificate, clave *rsa.PrivateKey, cadena ...*x509.Certificate) (*Certificado, error) {
	if cert == nil || clave == nil || !coincide(cert, clave) {
		return nil, ErrSinParDeFirma
	}
	return &Certificado{X509: cert, Cadena: cadena, clave: clave}, nil
}

func coincide(c *x509.Certificate, k *rsa.PrivateKey) bool {
	pub, ok := c.PublicKey.(*rsa.PublicKey)
	return ok && pub.Equal(&k.PublicKey)
}

// Firmante devuelve la clave privada como crypto.Signer. Es la única vía de
// acceso a la clave: permite firmar pero no obtener sus bytes.
//
// Devuelve nil (una interfaz nil, comparable con == nil) si el Certificado
// no tiene clave, por ejemplo un &Certificado{X509: …} construido a mano en
// vez de con Cargar o DesdeClave.
func (c *Certificado) Firmante() crypto.Signer {
	if c == nil || c.clave == nil {
		return nil
	}
	return c.clave
}

// Utilizable informa si el certificado tiene un X.509 y su clave privada,
// es decir, si sirve para firmar. Devuelve ErrSinParDeFirma si no.
func (c *Certificado) Utilizable() error {
	if c == nil || c.X509 == nil || c.clave == nil || !coincide(c.X509, c.clave) {
		return ErrSinParDeFirma
	}
	return nil
}

// ClavePublica devuelve la clave pública RSA del certificado, o nil si no
// tiene clave.
func (c *Certificado) ClavePublica() *rsa.PublicKey {
	if c == nil || c.clave == nil {
		return nil
	}
	return &c.clave.PublicKey
}

// Vigente comprueba que el certificado esté dentro de su período de validez
// en el instante dado y que su clave tenga al menos 2048 bits. Devuelve
// ErrSinParDeFirma si el certificado no tiene X.509 o clave.
func (c *Certificado) Vigente(en time.Time) error {
	if err := c.Utilizable(); err != nil {
		return err
	}
	if en.After(c.X509.NotAfter) {
		return fmt.Errorf("%w el %s", ErrVencido, c.X509.NotAfter.Format("2006-01-02"))
	}
	if en.Before(c.X509.NotBefore) {
		return fmt.Errorf("%w hasta el %s", ErrAunNoVigente, c.X509.NotBefore.Format("2006-01-02"))
	}
	if c.clave.N.BitLen() < 2048 {
		return fmt.Errorf("%w (%d)", ErrClaveDebil, c.clave.N.BitLen())
	}
	return nil
}

// Resumen son datos no sensibles del certificado, aptos para mostrar o
// registrar.
type Resumen struct {
	Titular string
	Emisor  string
	Serie   string
	Desde   time.Time
	Hasta   time.Time
	BitsRSA int
	RUC     string // si el certificado lo declara; puede estar vacío
}

// OID con los que algunas entidades de certificación ecuatorianas declaran
// el RUC del titular en extensiones privadas del certificado. La lista es de
// mejor esfuerzo: Resumen.RUC queda vacío si el certificado usa otro OID.
var oidsRUC = []string{
	"1.3.6.1.4.1.37746.3.11", // Security Data
	"1.3.6.1.4.1.37947.3.11", // Banco Central del Ecuador
	"1.3.6.1.4.1.37442.3.11", // ANF AC Ecuador
}

// Resumen describe el certificado sin exponer material privado.
func (c *Certificado) Resumen() Resumen {
	if c == nil || c.X509 == nil {
		return Resumen{}
	}
	bits := 0
	if c.clave != nil {
		bits = c.clave.N.BitLen()
	}
	r := Resumen{
		Titular: c.X509.Subject.CommonName,
		Emisor:  c.X509.Issuer.CommonName,
		Serie:   c.X509.SerialNumber.String(),
		Desde:   c.X509.NotBefore,
		Hasta:   c.X509.NotAfter,
		BitsRSA: bits,
	}
	for _, ext := range c.X509.Extensions {
		id := ext.Id.String()
		for _, o := range oidsRUC {
			if id == o {
				if ruc := extraerDigitos(ext.Value, 13); ruc != "" {
					r.RUC = ruc
				}
			}
		}
	}
	return r
}

// extraerDigitos busca en el valor DER de una extensión una secuencia de
// exactamente n dígitos.
func extraerDigitos(der []byte, n int) string {
	s := string(der)
	for i := 0; i+n <= len(s); i++ {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j-i == n {
			return s[i:j]
		}
		if j > i {
			i = j
		}
	}
	return ""
}

// String evita que un fmt.Printf("%v") accidental vuelque la clave privada.
func (c *Certificado) String() string {
	r := c.Resumen()
	return fmt.Sprintf("Certificado{titular=%q emisor=%q vence=%s}", r.Titular, r.Emisor, r.Hasta.Format("2006-01-02"))
}

// GoString evita lo mismo para %#v.
func (c *Certificado) GoString() string { return c.String() }

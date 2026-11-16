// Package xades firma y verifica comprobantes electrónicos con XAdES-BES
// (ETSI TS 101 903 v1.3.2) en modalidad enveloped, tal como exige la ficha
// técnica del SRI (sección 6 y Anexo 14).
//
// La implementación es Go puro y no depende de bibliotecas XML externas: usa
// un árbol y un canonicalizador C14N 1.0 propios (internal/xmldom).
//
// # Estructura de la firma
//
// Se reproduce la estructura del Anexo 14 de la ficha técnica v2.34, que
// es la que generan las bibliotecas que el SRI declara usar (MITyCLibXADES):
//
//	<ds:Signature Id>
//	  <ds:SignedInfo>
//	    <ds:Reference Type="…#SignedProperties" URI="#…-SignedProperties…"/>
//	    <ds:Reference URI="#Certificate…"/>            (KeyInfo, 6.5)
//	    <ds:Reference URI="#comprobante"> enveloped    (todo el comprobante)
//	  <ds:SignatureValue/>
//	  <ds:KeyInfo> X509Data + KeyValue
//	  <ds:Object> etsi:QualifyingProperties / SignedProperties
//
// # Algoritmos
//
// Por defecto se firma con RSA-SHA256, que el SRI autoriza (comprobado en
// su ambiente de certificación). La ficha técnica (6.8) nombra RSA-SHA1,
// disponible con Opciones{Algoritmo: SHA1}. Ver docs/decisiones.md (D2).
package xades

import (
	"crypto"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // exigido por la ficha técnica del SRI (6.8)
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/internal/xmldom"
)

// Espacios de nombres y URI de algoritmos.
const (
	NSDS    = "http://www.w3.org/2000/09/xmldsig#"
	NSXAdES = "http://uri.etsi.org/01903/v1.3.2#"

	AlgC14N          = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	AlgEnveloped     = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
	AlgRSASHA1       = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	AlgRSASHA256     = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	AlgRSASHA384     = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha384"
	AlgRSASHA512     = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha512"
	AlgSHA1          = "http://www.w3.org/2000/09/xmldsig#sha1"
	AlgSHA256        = "http://www.w3.org/2001/04/xmlenc#sha256"
	AlgSHA384        = "http://www.w3.org/2001/04/xmldsig-more#sha384"
	AlgSHA512        = "http://www.w3.org/2001/04/xmlenc#sha512"
	TipoSignedProps  = "http://uri.etsi.org/01903#SignedProperties"
	idRaizPorDefecto = "comprobante"
)

// Algoritmo selecciona la función resumen usada en la firma y en los
// digest de las referencias.
type Algoritmo int

const (
	// SHA256 es RSA-SHA256, el valor por defecto. El ambiente de
	// certificación del SRI autoriza comprobantes firmados así (prueba
	// real del 2026-11-14, docs/pruebas-sri.md).
	SHA256 Algoritmo = iota
	// SHA1 es RSA-SHA1, el algoritmo que nombra la ficha técnica (6.8) y
	// que usa su Anexo 14. El SRI también lo autoriza, pero SHA-1 está en
	// desuso criptográfico: úselo solo si un receptor lo exige.
	SHA1
)

func (a Algoritmo) String() string {
	if a == SHA1 {
		return "RSA-SHA1"
	}
	return "RSA-SHA256"
}

func (a Algoritmo) uris() (firma, digest string, h crypto.Hash) {
	if a == SHA1 {
		return AlgRSASHA1, AlgSHA1, crypto.SHA1
	}
	return AlgRSASHA256, AlgSHA256, crypto.SHA256
}

// Errores de firma.
var (
	// ErrYaFirmado indica que el documento ya contiene una ds:Signature.
	ErrYaFirmado = errors.New("xades: el documento ya está firmado")
	// ErrCertificado indica que falta el certificado o su clave.
	ErrCertificado = errors.New("xades: certificado de firma ausente")
)

// Opciones ajusta la firma. El valor cero es válido y firma con RSA-SHA256
// en el instante actual.
type Opciones struct {
	// Algoritmo de resumen; por defecto SHA256.
	Algoritmo Algoritmo
	// Momento es el valor de SigningTime; por defecto, ahora. Se escribe con
	// el desplazamiento horario de Ecuador continental (−05:00).
	Momento time.Time
	// PermitirVencido omite la comprobación de vigencia del certificado en
	// Momento. Solo tiene sentido en pruebas.
	PermitirVencido bool
}

// zonaEcuador es UTC−5, sin horario de verano.
var zonaEcuador = time.FixedZone("-05", -5*3600)

func digest(h crypto.Hash, datos []byte) []byte {
	switch h {
	case crypto.SHA256:
		s := sha256.Sum256(datos)
		return s[:]
	case crypto.SHA384:
		s := sha512.Sum384(datos)
		return s[:]
	case crypto.SHA512:
		s := sha512.Sum512(datos)
		return s[:]
	default:
		s := sha1.Sum(datos) //nolint:gosec
		return s[:]
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func idAleatorio() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000))
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

// Firmar añade una firma XAdES-BES enveloped al comprobante y devuelve el
// documento firmado.
//
// El documento devuelto se serializa en forma canónica, de manera que los
// bytes que se envían al SRI son exactamente los que cubre el digest. Antes
// de devolverlo, la firma se verifica con Verificar: nunca se entrega una
// firma que este mismo paquete no pueda validar.
//
// El elemento raíz debe ser el comprobante; si no tiene el atributo
// id="comprobante" se le añade, porque el XSD del SRI lo fija a ese valor.
func Firmar(xmlDoc []byte, cert *certificado.Certificado, op *Opciones) ([]byte, error) {
	if cert.Utilizable() != nil {
		return nil, ErrCertificado
	}
	var o Opciones
	if op != nil {
		o = *op
	}
	if o.Momento.IsZero() {
		o.Momento = time.Now()
	}
	if !o.PermitirVencido {
		if err := cert.Vigente(o.Momento); err != nil {
			return nil, err
		}
	}
	doc, err := xmldom.Analizar(xmlDoc)
	if err != nil {
		return nil, err
	}
	raiz := doc.Raiz
	yaFirmado := false
	raiz.Recorrer(func(e *xmldom.Elemento) bool {
		if e.Es(NSDS, "Signature") {
			yaFirmado = true
		}
		return !yaFirmado
	})
	if yaFirmado {
		return nil, ErrYaFirmado
	}
	idRaiz, ok := raiz.Attr("id")
	if !ok || idRaiz == "" {
		idRaiz = idRaizPorDefecto
		raiz.FijarAttr("id", idRaiz)
	}

	algFirma, algDigest, hash := o.Algoritmo.uris()

	var ids [8]string
	for i := range ids {
		if ids[i], err = idAleatorio(); err != nil {
			return nil, fmt.Errorf("xades: sin entropía: %w", err)
		}
	}
	idFirma := "Signature" + ids[0]
	idSignedInfo := "Signature-SignedInfo" + ids[1]
	idSignedProps := idFirma + "-SignedProperties" + ids[2]
	idKeyInfo := "Certificate" + ids[3]
	idRefDoc := "Reference-ID-" + ids[4]
	idRefProps := "SignedPropertiesID" + ids[5]
	idSigValue := "SignatureValue" + ids[6]
	idObject := idFirma + "-Object" + ids[7]

	ds := func(local string) *xmldom.Elemento { return xmldom.Nuevo("ds", local) }
	etsi := func(local string) *xmldom.Elemento { return xmldom.Nuevo("etsi", local) }
	metodo := func(local, alg string) *xmldom.Elemento { return ds(local).ConAttr("Algorithm", alg) }

	firma := ds("Signature").ConNS("ds", NSDS).ConNS("etsi", NSXAdES).ConAttr("Id", idFirma)

	// --- KeyInfo -----------------------------------------------------------
	pub := cert.ClavePublica()
	keyInfo := ds("KeyInfo").ConAttr("Id", idKeyInfo)
	keyInfo.Agregar(ds("X509Data")).Agregar(ds("X509Certificate").ConTexto(b64(cert.X509.Raw)))
	rsaKV := keyInfo.Agregar(ds("KeyValue")).Agregar(ds("RSAKeyValue"))
	rsaKV.Agregar(ds("Modulus").ConTexto(b64(pub.N.Bytes())))
	rsaKV.Agregar(ds("Exponent").ConTexto(b64(big.NewInt(int64(pub.E)).Bytes())))

	// --- Object / QualifyingProperties ------------------------------------
	objeto := ds("Object").ConAttr("Id", idObject)
	qp := objeto.Agregar(etsi("QualifyingProperties").ConAttr("Target", "#"+idFirma))
	sp := qp.Agregar(etsi("SignedProperties").ConAttr("Id", idSignedProps))
	ssp := sp.Agregar(etsi("SignedSignatureProperties"))
	ssp.Agregar(etsi("SigningTime").ConTexto(o.Momento.In(zonaEcuador).Format("2006-01-02T15:04:05-07:00")))
	certEl := ssp.Agregar(etsi("SigningCertificate")).Agregar(etsi("Cert"))
	cd := certEl.Agregar(etsi("CertDigest"))
	cd.Agregar(metodo("DigestMethod", algDigest))
	cd.Agregar(ds("DigestValue").ConTexto(b64(digest(hash, cert.X509.Raw))))
	is := certEl.Agregar(etsi("IssuerSerial"))
	emisor, err := NombreRFC2253(cert.X509.RawIssuer)
	if err != nil {
		return nil, fmt.Errorf("xades: emisor del certificado ilegible: %w", err)
	}
	is.Agregar(ds("X509IssuerName").ConTexto(emisor))
	is.Agregar(ds("X509SerialNumber").ConTexto(cert.X509.SerialNumber.String()))
	dof := sp.Agregar(etsi("SignedDataObjectProperties")).
		Agregar(etsi("DataObjectFormat").ConAttr("ObjectReference", "#"+idRefDoc))
	dof.Agregar(etsi("Description").ConTexto("contenido comprobante"))
	dof.Agregar(etsi("MimeType").ConTexto("text/xml"))

	// La firma se cuelga del documento antes de calcular los digest: la
	// canonicalización inclusiva escribe en cada vértice los espacios de
	// nombres heredados, y deben ser los que verá quien verifique.
	firma.Agregar(keyInfo)
	firma.Agregar(objeto)
	raiz.Agregar(firma)

	dDoc := digest(hash, xmldom.Canonicalizar(raiz, firma))
	dProps := digest(hash, xmldom.Canonicalizar(sp, nil))
	dKey := digest(hash, xmldom.Canonicalizar(keyInfo, nil))

	// --- SignedInfo ---------------------------------------------------------
	si := ds("SignedInfo").ConAttr("Id", idSignedInfo)
	si.Agregar(metodo("CanonicalizationMethod", AlgC14N))
	si.Agregar(metodo("SignatureMethod", algFirma))
	ref := func(e *xmldom.Elemento, d []byte, transformada bool) {
		if transformada {
			e.Agregar(ds("Transforms")).Agregar(metodo("Transform", AlgEnveloped))
		}
		e.Agregar(metodo("DigestMethod", algDigest))
		e.Agregar(ds("DigestValue").ConTexto(b64(d)))
		si.Agregar(e)
	}
	ref(ds("Reference").ConAttr("Id", idRefProps).ConAttr("Type", TipoSignedProps).ConAttr("URI", "#"+idSignedProps), dProps, false)
	ref(ds("Reference").ConAttr("URI", "#"+idKeyInfo), dKey, false)
	ref(ds("Reference").ConAttr("Id", idRefDoc).ConAttr("URI", "#"+idRaiz), dDoc, true)

	// Orden final de hijos: SignedInfo, SignatureValue, KeyInfo, Object.
	firma.Hijos = nil
	firma.Agregar(si)
	valor, err := cert.Firmante().Sign(rand.Reader, digest(hash, xmldom.Canonicalizar(si, nil)), hash)
	if err != nil {
		return nil, fmt.Errorf("xades: no se pudo firmar: %w", err)
	}
	firma.Agregar(ds("SignatureValue").ConAttr("Id", idSigValue).ConTexto(b64(valor)))
	firma.Agregar(keyInfo)
	firma.Agregar(objeto)

	firmado := doc.Serializar()
	if _, err := Verificar(firmado, &OpcionesVerificacion{Momento: o.Momento, omitirVigencia: o.PermitirVencido}); err != nil {
		return nil, fmt.Errorf("xades: la firma generada no supera la autoverificación: %w", err)
	}
	return firmado, nil
}

func hashDeFirma(alg string) (crypto.Hash, bool) {
	switch alg {
	case AlgRSASHA1:
		return crypto.SHA1, true
	case AlgRSASHA256:
		return crypto.SHA256, true
	case AlgRSASHA384:
		return crypto.SHA384, true
	case AlgRSASHA512:
		return crypto.SHA512, true
	}
	return 0, false
}

func hashDeDigest(alg string) (crypto.Hash, bool) {
	switch alg {
	case AlgSHA1:
		return crypto.SHA1, true
	case AlgSHA256:
		return crypto.SHA256, true
	case AlgSHA384:
		return crypto.SHA384, true
	case AlgSHA512:
		return crypto.SHA512, true
	}
	return 0, false
}

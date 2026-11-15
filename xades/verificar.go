package xades

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/internal/xmldom"
)

// Errores de verificación. Todos los fallos de Verificar envuelven uno de
// estos valores, de modo que errors.Is permite distinguir la causa.
var (
	// ErrSinFirma indica que el documento no contiene ds:Signature.
	ErrSinFirma = errors.New("xades: el documento no contiene una firma")
	// ErrVariasFirmas indica más de una ds:Signature; un comprobante del SRI
	// lleva exactamente una.
	ErrVariasFirmas = errors.New("xades: el documento contiene más de una firma")
	// ErrEstructura indica que falta un elemento obligatorio de la firma o
	// que está mal formado.
	ErrEstructura = errors.New("xades: estructura de firma inválida")
	// ErrAlgoritmo indica un algoritmo o transformación no admitidos.
	ErrAlgoritmo = errors.New("xades: algoritmo no admitido")
	// ErrDigest indica que el contenido referenciado fue alterado.
	ErrDigest = errors.New("xades: el digest de una referencia no coincide")
	// ErrFirmaInvalida indica que SignatureValue no corresponde a SignedInfo
	// con la clave del certificado.
	ErrFirmaInvalida = errors.New("xades: valor de firma inválido")
	// ErrReferencia indica una referencia que no se puede resolver o que es
	// ambigua (identificadores duplicados).
	ErrReferencia = errors.New("xades: referencia no resoluble")
	// ErrCobertura indica que la firma es criptográficamente correcta pero
	// no cubre lo que debe cubrir (el comprobante, las SignedProperties o
	// el KeyInfo).
	ErrCobertura = errors.New("xades: la firma no cubre el contenido exigido")
	// ErrCertificadoFirma indica un problema con el certificado incluido en
	// la firma: ausente, ilegible, distinto del declarado en
	// SigningCertificate, fuera de vigencia o sin cadena de confianza.
	ErrCertificadoFirma = errors.New("xades: certificado de la firma inválido")
)

// OpcionesVerificacion ajusta Verificar. El valor cero es válido.
type OpcionesVerificacion struct {
	// Raices, si no es nil, exige que el certificado firmante encadene
	// hasta una de estas autoridades (usando como intermedias los demás
	// certificados presentes en KeyInfo). Si es nil no se evalúa la
	// confianza: solo la integridad y la coherencia criptográfica.
	Raices *x509.CertPool
	// Momento en que debe estar vigente el certificado. Por defecto, el
	// SigningTime declarado en la firma; si no lo hay, el instante actual.
	Momento time.Time

	omitirVigencia bool
}

// Referencia describe una ds:Reference verificada.
type Referencia struct {
	URI      string
	Tipo     string
	Digest   string // URI del algoritmo
	Elemento string // nombre local del elemento resuelto
}

// Firma es el resultado de una verificación satisfactoria.
type Firma struct {
	// Certificado del firmante, tomado de KeyInfo y contrastado con
	// SigningCertificate.
	Certificado *x509.Certificate
	// FechaFirma es el SigningTime declarado por el firmante. No es un
	// sello de tiempo: XAdES-BES no lo certifica.
	FechaFirma time.Time
	// AlgoritmoFirma y Canonicalizacion son los URI declarados en SignedInfo.
	AlgoritmoFirma   string
	Canonicalizacion string
	// Referencias verificadas, en orden de documento.
	Referencias []Referencia
	// ID es el atributo Id de ds:Signature.
	ID string
}

// Verificar comprueba la firma XAdES-BES enveloped de un comprobante:
//
//  1. Cada ds:Reference: se resuelve, se le aplican sus transformaciones y
//     se compara su digest.
//  2. SignatureValue contra SignedInfo canonicalizado, con la clave pública
//     del certificado incluido en KeyInfo.
//  3. Cobertura: la firma debe abarcar el elemento raíz completo (con la
//     transformación enveloped), las SignedProperties de XAdES y el KeyInfo
//     (ficha técnica 6.4 y 6.5).
//  4. XAdES: el digest de SigningCertificate debe corresponder al
//     certificado de KeyInfo, lo que impide sustituirlo.
//  5. Vigencia del certificado en el momento de la firma y, si se dan
//     raíces, la cadena de confianza.
//
// Verificar no consulta listas de revocación ni OCSP.
func Verificar(xmlDoc []byte, op *OpcionesVerificacion) (*Firma, error) {
	var o OpcionesVerificacion
	if op != nil {
		o = *op
	}
	doc, err := xmldom.Analizar(xmlDoc)
	if err != nil {
		return nil, err
	}
	raiz := doc.Raiz

	var firmas []*xmldom.Elemento
	raiz.Recorrer(func(e *xmldom.Elemento) bool {
		if e.Es(NSDS, "Signature") {
			firmas = append(firmas, e)
			return false
		}
		return true
	})
	switch {
	case len(firmas) == 0:
		return nil, ErrSinFirma
	case len(firmas) > 1:
		return nil, ErrVariasFirmas
	}
	firma := firmas[0]
	if firma.Padre != raiz {
		return nil, fmt.Errorf("%w: la firma debe ser hija directa del elemento raíz", ErrEstructura)
	}
	res := &Firma{}
	res.ID, _ = firma.Attr("Id")

	si := firma.Hijo(NSDS, "SignedInfo")
	sv := firma.Hijo(NSDS, "SignatureValue")
	ki := firma.Hijo(NSDS, "KeyInfo")
	if si == nil || sv == nil || ki == nil {
		return nil, fmt.Errorf("%w: faltan SignedInfo, SignatureValue o KeyInfo", ErrEstructura)
	}

	// --- Algoritmos de SignedInfo ------------------------------------------
	cm := si.Hijo(NSDS, "CanonicalizationMethod")
	sm := si.Hijo(NSDS, "SignatureMethod")
	if cm == nil || sm == nil {
		return nil, fmt.Errorf("%w: SignedInfo incompleto", ErrEstructura)
	}
	res.Canonicalizacion, _ = cm.Attr("Algorithm")
	if res.Canonicalizacion != AlgC14N {
		return nil, fmt.Errorf("%w: canonicalización %q", ErrAlgoritmo, res.Canonicalizacion)
	}
	res.AlgoritmoFirma, _ = sm.Attr("Algorithm")
	hashFirma, ok := hashDeFirma(res.AlgoritmoFirma)
	if !ok {
		return nil, fmt.Errorf("%w: firma %q", ErrAlgoritmo, res.AlgoritmoFirma)
	}

	// --- Certificado ---------------------------------------------------------
	var certs []*x509.Certificate
	for _, xd := range ki.Elementos() {
		if !xd.Es(NSDS, "X509Data") {
			continue
		}
		for _, xc := range xd.Elementos() {
			if !xc.Es(NSDS, "X509Certificate") {
				continue
			}
			der, err := decodificarB64(xc.TextoPlano())
			if err != nil {
				return nil, fmt.Errorf("%w: X509Certificate no es base64", ErrCertificadoFirma)
			}
			c, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrCertificadoFirma, err)
			}
			certs = append(certs, c)
		}
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("%w: KeyInfo no contiene X509Certificate", ErrCertificadoFirma)
	}

	// --- XAdES: SignedProperties --------------------------------------------
	var sp, qp *xmldom.Elemento
	for _, obj := range firma.Elementos() {
		if !obj.Es(NSDS, "Object") {
			continue
		}
		if q := obj.Hijo(NSXAdES, "QualifyingProperties"); q != nil {
			if qp != nil {
				return nil, fmt.Errorf("%w: más de un QualifyingProperties", ErrEstructura)
			}
			qp = q
			sp = q.Hijo(NSXAdES, "SignedProperties")
		}
	}
	if sp == nil {
		return nil, fmt.Errorf("%w: falta etsi:SignedProperties (no es XAdES)", ErrEstructura)
	}
	if objetivo, _ := qp.Attr("Target"); res.ID == "" || objetivo != "#"+res.ID {
		return nil, fmt.Errorf("%w: QualifyingProperties/@Target no apunta a la firma", ErrEstructura)
	}

	// --- Referencias ---------------------------------------------------------
	var cubreRaiz, cubreProps, cubreKeyInfo bool
	nRefs := 0
	for _, ref := range si.Elementos() {
		if !ref.Es(NSDS, "Reference") {
			continue
		}
		nRefs++
		uri, tieneURI := ref.Attr("URI")
		tipo, _ := ref.Attr("Type")
		var destino *xmldom.Elemento
		switch {
		case !tieneURI:
			return nil, fmt.Errorf("%w: Reference sin URI", ErrReferencia)
		case uri == "":
			destino = raiz
		case strings.HasPrefix(uri, "#") && len(uri) > 1 && !strings.ContainsAny(uri[1:], "()# "):
			hallados := raiz.BuscarPorID(uri[1:])
			if len(hallados) != 1 {
				return nil, fmt.Errorf("%w: %q resuelve a %d elementos", ErrReferencia, uri, len(hallados))
			}
			destino = hallados[0]
		default:
			return nil, fmt.Errorf("%w: URI %q (solo se admiten referencias al mismo documento)", ErrReferencia, uri)
		}

		envuelta := false
		if ts := ref.Hijo(NSDS, "Transforms"); ts != nil {
			for _, tr := range ts.Elementos() {
				switch alg, _ := tr.Attr("Algorithm"); alg {
				case AlgEnveloped:
					envuelta = true
				case AlgC14N:
					// Es la canonicalización que ya se aplica por defecto.
				default:
					return nil, fmt.Errorf("%w: transformación %q", ErrAlgoritmo, alg)
				}
			}
		}
		dm := ref.Hijo(NSDS, "DigestMethod")
		dv := ref.Hijo(NSDS, "DigestValue")
		if dm == nil || dv == nil {
			return nil, fmt.Errorf("%w: Reference %q incompleta", ErrEstructura, uri)
		}
		algD, _ := dm.Attr("Algorithm")
		hD, ok := hashDeDigest(algD)
		if !ok {
			return nil, fmt.Errorf("%w: digest %q", ErrAlgoritmo, algD)
		}
		declarado, err := decodificarB64(dv.TextoPlano())
		if err != nil {
			return nil, fmt.Errorf("%w: DigestValue de %q no es base64", ErrEstructura, uri)
		}
		var excluir *xmldom.Elemento
		if envuelta {
			excluir = firma
		} else if contiene(destino, firma) {
			// Sin la transformación enveloped, una referencia que abarque
			// la propia firma no puede verificarse nunca.
			return nil, fmt.Errorf("%w: %q abarca la firma sin transformación enveloped", ErrReferencia, uri)
		}
		if !bytes.Equal(digest(hD, xmldom.Canonicalizar(destino, excluir)), declarado) {
			return nil, fmt.Errorf("%w: %q", ErrDigest, uri)
		}
		switch {
		case destino == raiz:
			cubreRaiz = true
		case destino == sp:
			if tipo != TipoSignedProps {
				return nil, fmt.Errorf("%w: la referencia a SignedProperties debe declarar Type=%q", ErrEstructura, TipoSignedProps)
			}
			cubreProps = true
		case destino == ki:
			cubreKeyInfo = true
		}
		res.Referencias = append(res.Referencias, Referencia{URI: uri, Tipo: tipo, Digest: algD, Elemento: destino.Local})
	}
	if nRefs == 0 {
		return nil, fmt.Errorf("%w: SignedInfo sin referencias", ErrEstructura)
	}

	// --- Valor de la firma ---------------------------------------------------
	valor, err := decodificarB64(sv.TextoPlano())
	if err != nil {
		return nil, fmt.Errorf("%w: SignatureValue no es base64", ErrEstructura)
	}
	hSI := digest(hashFirma, xmldom.Canonicalizar(si, nil))
	// El firmante es el certificado de KeyInfo cuya clave valida la firma.
	var firmante *x509.Certificate
	for _, c := range certs {
		if pub, ok := c.PublicKey.(*rsa.PublicKey); ok && rsa.VerifyPKCS1v15(pub, hashFirma, hSI, valor) == nil {
			firmante = c
			break
		}
	}
	if firmante == nil {
		return nil, ErrFirmaInvalida
	}
	res.Certificado = firmante

	// --- Cobertura -------------------------------------------------------------
	switch {
	case !cubreRaiz:
		return nil, fmt.Errorf("%w: ninguna referencia abarca el comprobante completo", ErrCobertura)
	case !cubreProps:
		return nil, fmt.Errorf("%w: las SignedProperties no están firmadas", ErrCobertura)
	}

	// --- SigningCertificate ----------------------------------------------------
	ssp := sp.Hijo(NSXAdES, "SignedSignatureProperties")
	if ssp == nil {
		return nil, fmt.Errorf("%w: falta SignedSignatureProperties", ErrEstructura)
	}
	certDeclarado := false
	if sc := ssp.Hijo(NSXAdES, "SigningCertificate"); sc != nil {
		for _, c := range sc.Elementos() {
			cdg := c.Ruta(NSXAdES, "CertDigest")
			if cdg == nil {
				continue
			}
			dm, dv := cdg.Hijo(NSDS, "DigestMethod"), cdg.Hijo(NSDS, "DigestValue")
			if dm == nil || dv == nil {
				continue
			}
			alg, _ := dm.Attr("Algorithm")
			h, ok := hashDeDigest(alg)
			if !ok {
				return nil, fmt.Errorf("%w: digest de certificado %q", ErrAlgoritmo, alg)
			}
			d, err := decodificarB64(dv.TextoPlano())
			if err == nil && bytes.Equal(d, digest(h, firmante.Raw)) {
				certDeclarado = true
			}
		}
	}
	// XAdES-BES exige proteger el certificado firmante: con
	// SigningCertificate o firmando el KeyInfo. El perfil del SRI usa ambos;
	// se exige al menos uno.
	if !certDeclarado && !cubreKeyInfo {
		return nil, fmt.Errorf("%w: el certificado firmante no está protegido por SigningCertificate ni por una referencia a KeyInfo", ErrCertificadoFirma)
	}
	if sc := ssp.Hijo(NSXAdES, "SigningCertificate"); sc != nil && !certDeclarado {
		return nil, fmt.Errorf("%w: SigningCertificate no corresponde al certificado que firmó", ErrCertificadoFirma)
	}

	if st := ssp.Hijo(NSXAdES, "SigningTime"); st != nil {
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(st.TextoPlano()))
		if err != nil {
			return nil, fmt.Errorf("%w: SigningTime ilegible", ErrEstructura)
		}
		res.FechaFirma = t
	}

	// --- Vigencia y confianza --------------------------------------------------
	momento := o.Momento
	if momento.IsZero() {
		momento = res.FechaFirma
	}
	if momento.IsZero() {
		momento = time.Now()
	}
	if !o.omitirVigencia && (momento.Before(firmante.NotBefore) || momento.After(firmante.NotAfter)) {
		return nil, fmt.Errorf("%w: no estaba vigente el %s (válido del %s al %s)", ErrCertificadoFirma,
			momento.Format("2006-01-02"), firmante.NotBefore.Format("2006-01-02"), firmante.NotAfter.Format("2006-01-02"))
	}
	if o.Raices != nil {
		inter := x509.NewCertPool()
		for _, c := range certs {
			if c != firmante {
				inter.AddCert(c)
			}
		}
		if _, err := firmante.Verify(x509.VerifyOptions{
			Roots: o.Raices, Intermediates: inter, CurrentTime: momento,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		}); err != nil {
			return nil, fmt.Errorf("%w: sin cadena de confianza: %v", ErrCertificadoFirma, err)
		}
	}
	return res, nil
}

func contiene(ancestro, e *xmldom.Elemento) bool {
	for x := e; x != nil; x = x.Padre {
		if x == ancestro {
			return true
		}
	}
	return false
}

// decodificarB64 acepta base64 con los espacios y saltos de línea que
// permite xsd:base64Binary.
func decodificarB64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
	if s == "" {
		return nil, errors.New("vacío")
	}
	return base64.StdEncoding.DecodeString(s)
}

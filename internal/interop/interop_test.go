// Package interop contrasta el SDK con implementaciones independientes:
// xmlsec1 (firma XMLDSig) y xmllint de libxml2 (C14N y XSD).
//
// Las pruebas se omiten si las herramientas no están instaladas, salvo con
// SRI_INTEROP=1, que las hace obligatorias (así corren en el CI):
//
//	sudo apt-get install xmlsec1 libxml2-utils
//	SRI_INTEROP=1 go test ./internal/interop
//
// Nacieron de la auditoría de v0.1.0, que encontró con estas mismas
// herramientas la diferencia de normalización de atributos (AUD-008).
package interop

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/internal/xmldom"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

func herramienta(t *testing.T, nombre string) string {
	t.Helper()
	ruta, err := exec.LookPath(nombre)
	if err != nil {
		if os.Getenv("SRI_INTEROP") == "1" {
			t.Fatalf("SRI_INTEROP=1 y falta %s", nombre)
		}
		t.Skipf("%s no está instalado", nombre)
	}
	return ruta
}

// verificarXmlsec devuelve nil si xmlsec1 da la firma por válida. caPEM es
// la autoridad de confianza: xmlsec1 1.3 toma la clave del certificado de
// KeyInfo y exige validar su cadena (sin búsqueda laxa de claves), y 1.2
// también la acepta así.
func verificarXmlsec(t *testing.T, xmlsec, caPEM string, doc []byte) error {
	t.Helper()
	f := filepath.Join(t.TempDir(), "doc.xml")
	if err := os.WriteFile(f, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	salida, err := exec.Command(xmlsec, "--verify", "--enabled-reference-uris", "same-doc",
		"--id-attr:id", "factura",
		"--id-attr:Id", "http://uri.etsi.org/01903/v1.3.2#:SignedProperties",
		"--id-attr:Id", "http://www.w3.org/2000/09/xmldsig#:KeyInfo",
		"--trusted-pem", caPEM, f).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, lastLine(salida))
	}
	return nil
}

func lastLine(b []byte) string {
	ls := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.Join(ls, " | ")
}

// La firma del SDK la valida xmlsec1, con ambos algoritmos y con textos que
// ejercitan el escapado; un documento alterado lo rechazan los dos.
func TestFirmaConXmlsec1(t *testing.T) {
	xmlsec := herramienta(t, "xmlsec1")
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	certPEM := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: m.CA.Raw}), 0o600)

	f := ejemplo.Factura(comprobante.NuevaFecha(2026, 11, 14), "000000001")
	f.InfoTributaria.RazonSocial = `EMISOR & HIJOS <S.A.> "Ñandú" 'áéí' €`
	f.InfoAdicional = []comprobante.CampoAdicional{{Nombre: "Correo \"x\" & <y>", Valor: "a@b.ec"}}
	doc, err := f.XML()
	if err != nil {
		t.Fatal(err)
	}
	for _, alg := range []xades.Algoritmo{xades.SHA256, xades.SHA1} {
		firmado, err := xades.Firmar(doc, m.Certificado, &xades.Opciones{Algoritmo: alg, Momento: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		if err := verificarXmlsec(t, xmlsec, certPEM, firmado); err != nil {
			t.Errorf("%s: xmlsec1 rechaza la firma del SDK: %v", alg, err)
		}
		alterado := bytes.Replace(firmado, []byte("<importeTotal>11.50"), []byte("<importeTotal>1.50"), 1)
		if bytes.Equal(alterado, firmado) {
			t.Fatal("la alteración no se aplicó")
		}
		if verificarXmlsec(t, xmlsec, certPEM, alterado) == nil {
			t.Errorf("%s: xmlsec1 aceptó un documento alterado", alg)
		}
		if _, err := xades.Verificar(alterado, nil); err == nil {
			t.Errorf("%s: xades.Verificar aceptó un documento alterado", alg)
		}
	}
}

// AUD-008: un salto de línea literal en un atributo. El SDK y xmlsec1 deben
// coincidir: la misma firma es válida para los dos o inválida para los dos.
func TestNormalizacionDeAtributosConXmlsec1(t *testing.T) {
	xmlsec := herramienta(t, "xmlsec1")
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	certPEM := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: m.CA.Raw}), 0o600)
	firmado, err := xades.Firmar([]byte(`<factura id="comprobante" version="1.1.0"><a b="x&#xA;y"/></factura>`), m.Certificado, &xades.Opciones{Momento: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	for nombre, doc := range map[string][]byte{
		"referencia de carácter": firmado,
		"salto literal":          bytes.Replace(firmado, []byte("x&#xA;y"), []byte("x\ny"), 1),
	} {
		_, errSDK := xades.Verificar(doc, nil)
		errLib := verificarXmlsec(t, xmlsec, certPEM, doc)
		if (errSDK == nil) != (errLib == nil) {
			t.Errorf("%s: el SDK dice %v y xmlsec1 %v", nombre, errSDK, errLib)
		}
	}
}

// generar escribe un documento aleatorio con espacios de nombres, atributos
// xml:*, referencias de carácter, CDATA y blancos en atributos.
func generar(r *rand.Rand, prof int, prefs []string, sb *strings.Builder) {
	textos := []string{"a", "&amp;", "&lt;", "&gt;", "&#xD;", "&#x9;", "&#xA;", "\"", "'", "ñ", "€", "𝄞", " ", "<![CDATA[x<&>]]>", "\t", "&#13;", "]]&gt;"}
	valores := []string{"a", "&amp;", "&lt;", "&#xD;", "&#x9;", "&#xA;", "'", "ñ", " ", "\t", "\n", "\r\n", "&quot;"}
	nom := []string{"a", "b", "c"}[r.IntN(3)]
	pre := ""
	if len(prefs) > 0 && r.IntN(3) == 0 {
		pre = prefs[r.IntN(len(prefs))] + ":"
	}
	sb.WriteString("<" + pre + nom)
	local := append([]string(nil), prefs...)
	if r.IntN(3) == 0 {
		p := []string{"p", "q", "r"}[r.IntN(3)]
		fmt.Fprintf(sb, " xmlns:%s=\"urn:%d\"", p, r.IntN(3))
		local = append(local, p)
	}
	if r.IntN(4) == 0 {
		fmt.Fprintf(sb, " xmlns=\"%s\"", []string{"", "urn:d", "urn:e"}[r.IntN(3)])
	}
	usados := map[string]bool{}
	for i := 0; i < r.IntN(4); i++ {
		an := []string{"x", "y", "id", "xml:lang", "xml:space"}[r.IntN(5)]
		if len(local) > 0 && r.IntN(3) == 0 && !strings.HasPrefix(an, "xml:") {
			an = local[r.IntN(len(local))] + ":" + an
		}
		if usados[an] {
			continue
		}
		usados[an] = true
		v := ""
		for j := 0; j < r.IntN(4); j++ {
			v += valores[r.IntN(len(valores))]
		}
		if an == "xml:space" {
			v = "preserve"
		}
		sb.WriteString(" " + an + "=\"" + v + "\"")
	}
	sb.WriteString(">")
	for i := 0; i < r.IntN(4); i++ {
		if prof < 5 && r.IntN(2) == 0 {
			generar(r, prof+1, local, sb)
		} else {
			sb.WriteString(textos[r.IntN(len(textos))])
		}
	}
	sb.WriteString("</" + pre + nom + ">")
}

// La forma canónica del SDK es, byte a byte, la de libxml2. La auditoría
// lo comprobó con 3000 documentos; aquí corren 400 con semilla fija.
func TestC14NConXmllint(t *testing.T) {
	xmllint := herramienta(t, "xmllint")
	r := rand.New(rand.NewPCG(1, 2))
	dir := t.TempDir()
	comparados := 0
	for i := 0; i < 400; i++ {
		var sb strings.Builder
		generar(r, 0, nil, &sb)
		d, err := xmldom.Analizar([]byte(sb.String()))
		if err != nil {
			continue
		}
		f := filepath.Join(dir, "doc.xml")
		os.WriteFile(f, []byte(sb.String()), 0o600)
		lib, err := exec.Command(xmllint, "--c14n", f).Output()
		if err != nil {
			t.Errorf("xmllint rechaza un documento que el SDK acepta: %q", sb.String())
			continue
		}
		if sdk := xmldom.Canonicalizar(d.Raiz, nil); !bytes.Equal(sdk, lib) {
			t.Errorf("C14N distinta\nentrada: %q\nSDK:     %q\nlibxml2: %q", sb.String(), sdk, lib)
		}
		comparados++
	}
	if comparados < 300 {
		t.Errorf("solo se compararon %d documentos", comparados)
	}
}

// archivoXSD devuelve el XSD con que se valida doc, según su raíz y versión.
func archivoXSD(t *testing.T, doc []byte) string {
	t.Helper()
	tipo, version, err := esquema.Identificar(doc)
	if err != nil {
		t.Fatal(err)
	}
	nombres := map[esquema.Tipo]string{
		esquema.Factura: "factura", esquema.NotaCredito: "NotaCredito", esquema.NotaDebito: "NotaDebito",
		esquema.ComprobanteRetencion: "ComprobanteRetencion", esquema.GuiaRemision: "GuiaRemision", esquema.LiquidacionCompra: "LiquidacionCompra",
	}
	return filepath.Join("..", "..", "esquema", "xsd", nombres[tipo]+"_V"+version+".xsd")
}

// El veredicto XSD del SDK (archivos literales del SRI) coincide con el de
// xmllint en los ejemplos oficiales, en un comprobante del SDK y en
// variantes inválidas de él.
func TestXSDConXmllint(t *testing.T) {
	xmllint := herramienta(t, "xmllint")
	casos := map[string][]byte{}
	ejemplos, _ := filepath.Glob(filepath.Join("..", "..", "esquema", "testdata", "*.xml"))
	for _, f := range ejemplos {
		doc, _ := os.ReadFile(f)
		base := filepath.Base(f)
		version := strings.TrimSuffix(base[strings.Index(base, "_V")+2:], ".xml")
		casos[base] = []byte(strings.Replace(string(doc), `version="version0"`, `version="`+version+`"`, 1))
	}
	valido, err := ejemplo.Factura(comprobante.NuevaFecha(2026, 11, 14), "000000001").XML()
	if err != nil {
		t.Fatal(err)
	}
	casos["factura del SDK"] = valido
	for nombre, cambio := range map[string][2]string{
		"sin ambiente":           {"<ambiente>1</ambiente>", ""},
		"letras en un importe":   {"<importeTotal>11.50</importeTotal>", "<importeTotal>once</importeTotal>"},
		"elemento desconocido":   {"</infoTributaria>", "<extra/></infoTributaria>"},
		"orden alterado":         {"<tipoEmision>1</tipoEmision>", ""},
		"demasiados decimales":   {"<importeTotal>11.50</importeTotal>", "<importeTotal>11.505</importeTotal>"},
		"razón social de 301":    {"<razonSocial>EMISOR DE PRUEBA S.A.</razonSocial>", "<razonSocial>" + strings.Repeat("A", 301) + "</razonSocial>"},
		"codDoc fuera de patrón": {"<codDoc>01</codDoc>", "<codDoc>1</codDoc>"},
	} {
		mod := bytes.Replace(valido, []byte(cambio[0]), []byte(cambio[1]), 1)
		if bytes.Equal(mod, valido) {
			t.Fatalf("%s: la variante no cambió el documento", nombre)
		}
		casos["factura del SDK, "+nombre] = mod
	}
	dir := t.TempDir()
	for nombre, doc := range casos {
		f := filepath.Join(dir, "doc.xml")
		os.WriteFile(f, doc, 0o600)
		errLib := exec.Command(xmllint, "--noout", "--schema", archivoXSD(t, doc), f).Run()
		errSDK := esquema.ValidarEstricto(doc)
		if (errSDK == nil) != (errLib == nil) {
			t.Errorf("%s: el SDK dice %v y xmllint %v", nombre, errSDK, errLib)
		}
	}
}

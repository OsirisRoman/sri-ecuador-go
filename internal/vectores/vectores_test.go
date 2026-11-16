// Package vectores genera y comprueba los vectores de prueba del directorio
// vectores/ del repositorio, pensados para quien implemente el protocolo
// del SRI en cualquier lenguaje.
//
// Las pruebas normales comprueban que los archivos versionados siguen
// siendo coherentes con el SDK. Para regenerarlos (cambia el certificado
// desechable y, con él, las firmas):
//
//	GENERAR_VECTORES=1 go test ./internal/vectores
package vectores

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/xmldom"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

const dir = "../../vectores"

var momento = time.Date(2026, 11, 14, 10, 0, 0, 0, time.FixedZone("-05", -5*3600))

type casoClave struct {
	Descripcion string `json:"descripcion"`
	// Componentes en el orden de la Tabla 1.
	FechaEmision    string `json:"fechaEmision"` // dd/mm/aaaa
	TipoComprobante string `json:"tipoComprobante"`
	RUC             string `json:"ruc"`
	Ambiente        string `json:"ambiente"`
	Establecimiento string `json:"establecimiento"`
	PuntoEmision    string `json:"puntoEmision"`
	Secuencial      string `json:"secuencial"`
	CodigoNumerico  string `json:"codigoNumerico"`
	TipoEmision     string `json:"tipoEmision"`
	// Resultado.
	Base48            string `json:"base48"`
	DigitoVerificador int    `json:"digitoVerificador"`
	ClaveAcceso       string `json:"claveAcceso"`
}

func casosClave(t *testing.T) []casoClave {
	t.Helper()
	entradas := []casoClave{
		{Descripcion: "Anexo 14 de la ficha técnica v2.34", FechaEmision: "05/03/2012", TipoComprobante: "01", RUC: "1760013210001", Ambiente: "1", Establecimiento: "001", PuntoEmision: "003", Secuencial: "000990064", CodigoNumerico: "12345678"},
		{Descripcion: "factura en producción", FechaEmision: "31/12/2026", TipoComprobante: "01", RUC: "1790000000001", Ambiente: "2", Establecimiento: "002", PuntoEmision: "010", Secuencial: "000000001", CodigoNumerico: "00000001"},
		{Descripcion: "nota de crédito", FechaEmision: "01/01/2027", TipoComprobante: "04", RUC: "1790000000001", Ambiente: "1", Establecimiento: "001", PuntoEmision: "001", Secuencial: "000000123", CodigoNumerico: "87654321"},
		{Descripcion: "comprobante de retención", FechaEmision: "29/02/2028", TipoComprobante: "07", RUC: "0990000000001", Ambiente: "1", Establecimiento: "001", PuntoEmision: "001", Secuencial: "999999999", CodigoNumerico: "99999999"},
		{Descripcion: "guía de remisión", FechaEmision: "15/11/2026", TipoComprobante: "06", RUC: "1790000000001", Ambiente: "1", Establecimiento: "003", PuntoEmision: "002", Secuencial: "000004567", CodigoNumerico: "13572468"},
	}
	// Se añaden casos hasta cubrir los dos resultados especiales del módulo
	// 11: resto 0 (11 → dígito 0) y resto 1 (10 → dígito 1).
	visto := map[int]bool{}
	for i := 1; len(visto) < 2 && i < 500; i++ {
		c := casoClave{Descripcion: "caso límite del módulo 11", FechaEmision: "14/11/2026", TipoComprobante: "01", RUC: "1790000000001", Ambiente: "1",
			Establecimiento: "001", PuntoEmision: "001", Secuencial: strings.Repeat("0", 6) + pad3(i), CodigoNumerico: "12345678"}
		completar(t, &c)
		if (c.DigitoVerificador == 0 || c.DigitoVerificador == 1) && !visto[c.DigitoVerificador] {
			visto[c.DigitoVerificador] = true
			if c.DigitoVerificador == 0 {
				c.Descripcion += ": 11 − resto = 11, el dígito es 0"
			} else {
				c.Descripcion += ": 11 − resto = 10, el dígito es 1"
			}
			entradas = append(entradas, c)
		}
	}
	for i := range entradas {
		completar(t, &entradas[i])
	}
	return entradas
}

func pad3(n int) string {
	s := "00" + itoa(n)
	return s[len(s)-3:]
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func completar(t *testing.T, c *casoClave) {
	t.Helper()
	f, err := comprobante.ParseFecha(c.FechaEmision)
	if err != nil {
		t.Fatal(err)
	}
	c.TipoEmision = "1"
	k, err := clave.Generar(clave.Componentes{FechaEmision: f.Time(), TipoComprobante: c.TipoComprobante, RUC: c.RUC, Ambiente: c.Ambiente,
		Establecimiento: c.Establecimiento, PuntoEmision: c.PuntoEmision, Secuencial: c.Secuencial, CodigoNumerico: c.CodigoNumerico})
	if err != nil {
		t.Fatal(err)
	}
	c.ClaveAcceso, c.Base48, c.DigitoVerificador = k, k[:48], int(k[48]-'0')
}

type lineaCalculo struct {
	Cantidad       string `json:"cantidad"`
	PrecioUnitario string `json:"precioUnitario"`
	Descuento      string `json:"descuento"`
	CodigoIVA      string `json:"codigoPorcentajeIVA"`
	// Resultados.
	PrecioTotalSinImpuesto string `json:"precioTotalSinImpuesto"`
	Tarifa                 string `json:"tarifa"`
	BaseImponible          string `json:"baseImponible"`
	Valor                  string `json:"valor"`
}

type totalCalculo struct {
	CodigoIVA     string `json:"codigoPorcentajeIVA"`
	BaseImponible string `json:"baseImponible"`
	Valor         string `json:"valor"`
}

type casoCalculo struct {
	Descripcion       string         `json:"descripcion"`
	Propina           string         `json:"propina"`
	Lineas            []lineaCalculo `json:"lineas"`
	TotalSinImpuestos string         `json:"totalSinImpuestos"`
	TotalDescuento    string         `json:"totalDescuento"`
	TotalConImpuestos []totalCalculo `json:"totalConImpuestos"`
	ImporteTotal      string         `json:"importeTotal"`
}

func casosCalculo() []casoCalculo {
	casos := []casoCalculo{
		{Descripcion: "una línea con IVA 15 %", Lineas: []lineaCalculo{{Cantidad: "2", PrecioUnitario: "1.75", CodigoIVA: "4"}}},
		{Descripcion: "seis decimales y descuento; IVA 15 % y 0 %", Lineas: []lineaCalculo{
			{Cantidad: "2.5", PrecioUnitario: "19.123456", Descuento: "3.21", CodigoIVA: "4"},
			{Cantidad: "3", PrecioUnitario: "12.50", CodigoIVA: "0"}}},
		{Descripcion: "redondeo mitad hacia arriba en la línea (0.005 → 0.01)", Lineas: []lineaCalculo{{Cantidad: "1", PrecioUnitario: "0.005", CodigoIVA: "0"}}},
		{Descripcion: "el IVA se calcula sobre la base acumulada del grupo, no sumando el de cada línea (3 × 0.33)", Lineas: []lineaCalculo{
			{Cantidad: "1", PrecioUnitario: "0.33", CodigoIVA: "4"}, {Cantidad: "1", PrecioUnitario: "0.33", CodigoIVA: "4"}, {Cantidad: "1", PrecioUnitario: "0.33", CodigoIVA: "4"}}},
		{Descripcion: "exento, no objeto, 5 % y propina", Propina: "1.00", Lineas: []lineaCalculo{
			{Cantidad: "1", PrecioUnitario: "8.00", CodigoIVA: "7"}, {Cantidad: "1", PrecioUnitario: "4.00", CodigoIVA: "6"},
			{Cantidad: "10", PrecioUnitario: "1.10", CodigoIVA: "5"}, {Cantidad: "1", PrecioUnitario: "10.00", CodigoIVA: "4"}}},
	}
	for i := range casos {
		c := &casos[i]
		f := &factura.Factura{}
		if c.Propina != "" {
			f.InfoFactura.Propina = decimal.MustParse(c.Propina)
		} else {
			c.Propina = "0.00"
		}
		for j := range c.Lineas {
			l := &c.Lineas[j]
			d := factura.LineaIVA("X", "x", decimal.MustParse(l.Cantidad), decimal.MustParse(l.PrecioUnitario), l.CodigoIVA)
			if l.Descuento != "" {
				d.Descuento = decimal.MustParse(l.Descuento)
			} else {
				l.Descuento = "0.00"
			}
			f.Detalles = append(f.Detalles, d)
		}
		f.Calcular()
		for j, d := range f.Detalles {
			l := &c.Lineas[j]
			imp := d.Impuestos[0]
			l.PrecioTotalSinImpuesto, l.Tarifa, l.BaseImponible, l.Valor = d.PrecioTotalSinImpuesto.String(), imp.Tarifa.String(), imp.BaseImponible.String(), imp.Valor.String()
		}
		i := f.InfoFactura
		c.TotalSinImpuestos, c.TotalDescuento, c.ImporteTotal = i.TotalSinImpuestos.String(), i.TotalDescuento.String(), i.ImporteTotal.String()
		c.TotalConImpuestos = nil
		for _, t := range i.TotalConImpuestos {
			c.TotalConImpuestos = append(c.TotalConImpuestos, totalCalculo{t.CodigoPorcentaje, t.BaseImponible.String(), t.Valor.String()})
		}
	}
	return casos
}

// datosFirma describe una firma de los vectores para poder comprobar cada
// paso por separado.
type datosFirma struct {
	Nota                  string `json:"nota"`
	Algoritmo             string `json:"algoritmo"`
	SigningTime           string `json:"signingTime"`
	EmisorDERHex          string `json:"emisorDelCertificadoDERHex"`
	X509IssuerName        string `json:"x509IssuerName"`
	X509SerialNumber      string `json:"x509SerialNumber"`
	DigestComprobante     string `json:"digestComprobante"`
	DigestSignedProps     string `json:"digestSignedProperties"`
	DigestKeyInfo         string `json:"digestKeyInfo"`
	ArchivoComprobanteC14 string `json:"c14nComprobante"`
	ArchivoSignedPropsC14 string `json:"c14nSignedProperties"`
	ArchivoKeyInfoC14     string `json:"c14nKeyInfo"`
	ArchivoSignedInfoC14  string `json:"c14nSignedInfo"`
}

func b64sha256(b []byte) string {
	s := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(s[:])
}

// piezas canonicaliza las cuatro partes de un documento firmado.
func piezas(t *testing.T, firmado []byte) (doc, props, keyInfo, signedInfo []byte) {
	t.Helper()
	d, err := xmldom.Analizar(firmado)
	if err != nil {
		t.Fatal(err)
	}
	firma := d.Raiz.Hijo(xades.NSDS, "Signature")
	sp := firma.Ruta(xades.NSDS, "Object").Ruta(xades.NSXAdES, "QualifyingProperties", "SignedProperties")
	return xmldom.Canonicalizar(d.Raiz, firma), xmldom.Canonicalizar(sp, nil),
		xmldom.Canonicalizar(firma.Hijo(xades.NSDS, "KeyInfo"), nil), xmldom.Canonicalizar(firma.Hijo(xades.NSDS, "SignedInfo"), nil)
}

func escribir(t *testing.T, nombre string, datos []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, nombre)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nombre), datos, 0o644); err != nil {
		t.Fatal(err)
	}
}

func escribirJSON(t *testing.T, nombre string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	escribir(t, nombre, append(b, '\n'))
}

func leer(t *testing.T, nombre string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, nombre))
	if err != nil {
		t.Fatalf("falta el vector %s (regenere con GENERAR_VECTORES=1): %v", nombre, err)
	}
	return b
}

func TestGenerar(t *testing.T) {
	if os.Getenv("GENERAR_VECTORES") == "" {
		t.Skip("defina GENERAR_VECTORES=1 para regenerar los vectores")
	}
	escribirJSON(t, "clave-acceso.json", casosClave(t))
	escribirJSON(t, "calculo-factura.json", casosCalculo())
	escribirJSON(t, "precio-final.json", casosPrecioFinal())

	f := &factura.Factura{
		InfoTributaria: comprobante.InfoTributaria{Ambiente: comprobante.Pruebas, RazonSocial: "EMISOR FICTICIO S.A.", NombreComercial: "Ñandú & Cía.",
			RUC: "1790000000001", Estab: "001", PtoEmi: "001", Secuencial: "000000001", DirMatriz: "Av. Amazonas N34-10 y Roca"},
		InfoFactura: factura.InfoFactura{FechaEmision: comprobante.NuevaFecha(2026, 11, 14), ObligadoContabilidad: "SI",
			TipoIdentificacionComprador: comprobante.IDRuc, RazonSocialComprador: "PRUEBAS SERVICIO DE RENTAS INTERNAS", IdentificacionComprador: "1760013210001",
			Pagos: []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}},
		Detalles: []factura.Detalle{
			factura.LineaIVA("SRV-001", "Servicio de diseño & desarrollo <web>", decimal.MustParse("2.5"), decimal.MustParse("19.123456"), comprobante.IVA15),
			factura.LineaIVA("LIB-002", "Libro técnico (tarifa 0 %)", decimal.Entero(3), decimal.MustParse("12.50"), comprobante.IVA0),
		},
		InfoAdicional: []comprobante.CampoAdicional{{Nombre: "Observación", Valor: `"comillas" <ángulos> 'apóstrofo' & ampersand`}},
	}
	f.Calcular()
	if _, err := f.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	sinFirma, err := f.XML()
	if err != nil {
		t.Fatal(err)
	}
	escribir(t, "firma/factura.xml", sinFirma)
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	for _, alg := range []xades.Algoritmo{xades.SHA256, xades.SHA1} {
		firmado, err := xades.Firmar(sinFirma, m.Certificado, &xades.Opciones{Algoritmo: alg, Momento: momento})
		if err != nil {
			t.Fatal(err)
		}
		sufijo := "sha256"
		if alg == xades.SHA1 {
			sufijo = "sha1"
		}
		escribir(t, "firma/factura.firmada."+sufijo+".xml", firmado)
		if alg != xades.SHA256 {
			continue
		}
		doc, props, ki, si := piezas(t, firmado)
		escribir(t, "firma/c14n-comprobante.xml", doc)
		escribir(t, "firma/c14n-signedproperties.xml", props)
		escribir(t, "firma/c14n-keyinfo.xml", ki)
		escribir(t, "firma/c14n-signedinfo.xml", si)
		emisor, err := xades.NombreRFC2253(m.Certificado.X509.RawIssuer)
		if err != nil {
			t.Fatal(err)
		}
		escribirJSON(t, "firma/firma.sha256.json", datosFirma{
			Nota:         "Datos intermedios de factura.firmada.sha256.xml. El certificado es desechable y lo emite una autoridad ficticia.",
			Algoritmo:    alg.String(),
			SigningTime:  momento.Format("2006-01-02T15:04:05-07:00"),
			EmisorDERHex: hex.EncodeToString(m.Certificado.X509.RawIssuer), X509IssuerName: emisor,
			X509SerialNumber:  m.Certificado.X509.SerialNumber.String(),
			DigestComprobante: b64sha256(doc), DigestSignedProps: b64sha256(props), DigestKeyInfo: b64sha256(ki),
			ArchivoComprobanteC14: "c14n-comprobante.xml", ArchivoSignedPropsC14: "c14n-signedproperties.xml",
			ArchivoKeyInfoC14: "c14n-keyinfo.xml", ArchivoSignedInfoC14: "c14n-signedinfo.xml",
		})
	}
}

func TestClaveAcceso(t *testing.T) {
	var casos []casoClave
	if err := json.Unmarshal(leer(t, "clave-acceso.json"), &casos); err != nil {
		t.Fatal(err)
	}
	if len(casos) < 5 {
		t.Fatalf("solo %d casos", len(casos))
	}
	digitos := map[int]bool{}
	for _, c := range casos {
		quiero := c
		completar(t, &c)
		if c != quiero {
			t.Errorf("%s: el SDK genera %s y el vector dice %s", quiero.Descripcion, c.ClaveAcceso, quiero.ClaveAcceso)
		}
		if err := clave.Validar(quiero.ClaveAcceso); err != nil {
			t.Errorf("%s: %v", quiero.Descripcion, err)
		}
		digitos[quiero.DigitoVerificador] = true
	}
	if !digitos[0] || !digitos[1] {
		t.Error("faltan los casos límite del módulo 11 (dígitos 0 y 1)")
	}
}

func TestCalculo(t *testing.T) {
	var guardados []casoCalculo
	if err := json.Unmarshal(leer(t, "calculo-factura.json"), &guardados); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(guardados)
	b, _ := json.Marshal(casosCalculo())
	if string(a) != string(b) {
		t.Errorf("los vectores de cálculo ya no coinciden con el SDK:\n vector: %s\n    SDK: %s", a, b)
	}
}

// lineaPrecioFinal es un detalle definido por su precio final con IVA
// incluido: las entradas y todo lo que se deduce de ellas.
type lineaPrecioFinal struct {
	Cantidad    string `json:"cantidad"`
	PrecioFinal string `json:"precioFinal"`
	Descuento   string `json:"descuento"`
	CodigoIVA   string `json:"codigoPorcentajeIVA"`
	// Resultados.
	ImporteFinal           string `json:"importeFinal"`
	Tarifa                 string `json:"tarifa"`
	BaseImponible          string `json:"baseImponible"`
	Valor                  string `json:"valor"`
	PrecioUnitario         string `json:"precioUnitario"`
	PrecioTotalSinImpuesto string `json:"precioTotalSinImpuesto"`
	// ValorSobreLaBase es el IVA que daría la misma base definida por su
	// precio sin impuestos: redondear(base × tarifa / 100, 2).
	ValorSobreLaBase string `json:"valorSobreLaBase"`
}

type casoPrecioFinal struct {
	Descripcion       string             `json:"descripcion"`
	Lineas            []lineaPrecioFinal `json:"lineas"`
	TotalSinImpuestos string             `json:"totalSinImpuestos"`
	TotalConImpuestos []totalCalculo     `json:"totalConImpuestos"`
	ImporteTotal      string             `json:"importeTotal"`
}

func casosPrecioFinal() []casoPrecioFinal {
	diez := func(l lineaPrecioFinal) []lineaPrecioFinal {
		out := make([]lineaPrecioFinal, 10)
		for i := range out {
			out[i] = l
		}
		return out
	}
	casos := []casoPrecioFinal{
		{Descripcion: "12,00 al 15 %: ningún precio sin impuestos de 2 decimales da este total", Lineas: []lineaPrecioFinal{{Cantidad: "1", PrecioFinal: "12.00", CodigoIVA: "4"}}},
		{Descripcion: "35,00 al 15 %", Lineas: []lineaPrecioFinal{{Cantidad: "1", PrecioFinal: "35.00", CodigoIVA: "4"}}},
		{Descripcion: "0,50 al 15 %", Lineas: []lineaPrecioFinal{{Cantidad: "1", PrecioFinal: "0.50", CodigoIVA: "4"}}},
		{Descripcion: "350,00 al 15 %: los dos modos coinciden", Lineas: []lineaPrecioFinal{{Cantidad: "1", PrecioFinal: "350.00", CodigoIVA: "4"}}},
		{Descripcion: "diez líneas de 0,50: el IVA total es la suma de las líneas (0,70), no la tarifa sobre la base acumulada (0,65)", Lineas: diez(lineaPrecioFinal{Cantidad: "1", PrecioFinal: "0.50", CodigoIVA: "4"})},
		{Descripcion: "cantidad 3: el precio unitario sin impuestos tiene 6 decimales", Lineas: []lineaPrecioFinal{{Cantidad: "3", PrecioFinal: "12.00", CodigoIVA: "4"}}},
		{Descripcion: "cantidad con decimales: el importe final se redondea primero (24,975 → 24,98)", Lineas: []lineaPrecioFinal{{Cantidad: "2.5", PrecioFinal: "9.99", CodigoIVA: "4"}}},
		{Descripcion: "con descuento sin impuestos de 5,00: solo cambia el precio unitario", Lineas: []lineaPrecioFinal{{Cantidad: "1", PrecioFinal: "12.00", Descuento: "5.00", CodigoIVA: "4"}}},
		{Descripcion: "tarifas 5 % y 0 %, y una línea al 15 %", Lineas: []lineaPrecioFinal{
			{Cantidad: "1", PrecioFinal: "12.00", CodigoIVA: "5"}, {Cantidad: "1", PrecioFinal: "12.00", CodigoIVA: "0"}, {Cantidad: "2", PrecioFinal: "35.00", CodigoIVA: "4"}}},
	}
	for i := range casos {
		c := &casos[i]
		f := &factura.Factura{}
		for j := range c.Lineas {
			l := &c.Lineas[j]
			d := factura.LineaPrecioFinal("X", "x", decimal.MustParse(l.Cantidad), decimal.MustParse(l.PrecioFinal), l.CodigoIVA)
			if l.Descuento != "" {
				d.Descuento = decimal.MustParse(l.Descuento)
			} else {
				l.Descuento = "0.00"
			}
			f.Detalles = append(f.Detalles, d)
		}
		f.Calcular()
		for j, d := range f.Detalles {
			l := &c.Lineas[j]
			imp := d.Impuestos[0]
			l.ImporteFinal = imp.BaseImponible.Sumar(imp.Valor).String()
			l.Tarifa, l.BaseImponible, l.Valor = imp.Tarifa.String(), imp.BaseImponible.String(), imp.Valor.String()
			l.PrecioUnitario, l.PrecioTotalSinImpuesto = d.PrecioUnitario.String(), d.PrecioTotalSinImpuesto.String()
			l.ValorSobreLaBase = imp.BaseImponible.Porcentaje(imp.Tarifa).Redondear(2).String()
		}
		i := f.InfoFactura
		c.TotalSinImpuestos, c.ImporteTotal = i.TotalSinImpuestos.String(), i.ImporteTotal.String()
		c.TotalConImpuestos = nil
		for _, t := range i.TotalConImpuestos {
			c.TotalConImpuestos = append(c.TotalConImpuestos, totalCalculo{t.CodigoPorcentaje, t.BaseImponible.String(), t.Valor.String()})
		}
	}
	return casos
}

func TestPrecioFinal(t *testing.T) {
	if os.Getenv("GENERAR_VECTORES") == "precio-final" {
		escribirJSON(t, "precio-final.json", casosPrecioFinal())
	}
	var guardados []casoPrecioFinal
	if err := json.Unmarshal(leer(t, "precio-final.json"), &guardados); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(guardados)
	b, _ := json.Marshal(casosPrecioFinal())
	if string(a) != string(b) {
		t.Errorf("los vectores de precio final ya no coinciden con el SDK:\n vector: %s\n    SDK: %s", a, b)
	}
}

func TestFirma(t *testing.T) {
	sinFirma := leer(t, "firma/factura.xml")
	if err := esquema.ValidarEstricto(sinFirma); err != nil {
		t.Errorf("factura.xml no cumple el XSD: %v", err)
	}
	for _, sufijo := range []string{"sha256", "sha1"} {
		firmado := leer(t, "firma/factura.firmada."+sufijo+".xml")
		if _, err := xades.Verificar(firmado, &xades.OpcionesVerificacion{Momento: momento}); err != nil {
			t.Errorf("%s: la firma del vector no verifica: %v", sufijo, err)
		}
		if err := esquema.ValidarEstricto(firmado); err != nil {
			t.Errorf("%s: el documento firmado no cumple el XSD: %v", sufijo, err)
		}
		if !strings.HasPrefix(string(firmado), strings.TrimSuffix(strings.NewReplacer("&#34;", `"`, "&#39;", "'").Replace(string(sinFirma)), "</factura>")) {
			t.Errorf("%s: el documento firmado no empieza por el comprobante sin firma", sufijo)
		}
	}
	firmado := leer(t, "firma/factura.firmada.sha256.xml")
	var d datosFirma
	if err := json.Unmarshal(leer(t, "firma/firma.sha256.json"), &d); err != nil {
		t.Fatal(err)
	}
	doc, props, ki, si := piezas(t, firmado)
	for nombre, par := range map[string][2][]byte{
		d.ArchivoComprobanteC14: {doc, []byte(d.DigestComprobante)}, d.ArchivoSignedPropsC14: {props, []byte(d.DigestSignedProps)},
		d.ArchivoKeyInfoC14: {ki, []byte(d.DigestKeyInfo)}, d.ArchivoSignedInfoC14: {si, nil},
	} {
		if string(leer(t, "firma/"+nombre)) != string(par[0]) {
			t.Errorf("%s no coincide con la forma canónica que calcula el SDK", nombre)
		}
		if par[1] != nil {
			if dg := b64sha256(par[0]); dg != string(par[1]) || !strings.Contains(string(si), "<ds:DigestValue>"+dg+"</ds:DigestValue>") {
				t.Errorf("%s: el digest %s no es el declarado o no figura en SignedInfo", nombre, dg)
			}
		}
	}
	der, _ := hex.DecodeString(d.EmisorDERHex)
	if nombre, err := xades.NombreRFC2253(der); err != nil || nombre != d.X509IssuerName || !strings.Contains(string(firmado), "<ds:X509IssuerName>"+nombre+"</ds:X509IssuerName>") {
		t.Errorf("X509IssuerName = %q, %v", nombre, err)
	}
}

package ride_test

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/ride"
)

// contenido descomprime los flujos del PDF y devuelve su texto, y el número
// de páginas.
func contenido(t *testing.T, pdf []byte) (string, int) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatal("no es un PDF")
	}
	var sb strings.Builder
	resto := pdf
	for {
		i := bytes.Index(resto, []byte("\nstream\n"))
		if i < 0 {
			break
		}
		resto = resto[i+8:]
		j := bytes.Index(resto, []byte("\nendstream"))
		if z, err := zlib.NewReader(bytes.NewReader(resto[:j])); err == nil {
			b, _ := io.ReadAll(z)
			sb.Write(b)
		}
		resto = resto[j:]
	}
	return sb.String(), bytes.Count(pdf, []byte("/Type /Page /Parent"))
}

func ejemploOficial(t *testing.T, archivo, version string) []byte {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", archivo))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.Replace(string(doc), `version="version0"`, `version="`+version+`"`, 1))
}

func TestTodosLosTipos(t *testing.T) {
	casos := []struct{ archivo, version, titulo, tipo string }{
		{"factura_V2.1.0.xml", "2.1.0", "FACTURA", "Factura"},
		{"NotaCredito_V1.1.0.xml", "1.1.0", "NOTA DE CR\xc9DITO", "Nota de crédito"},
		{"NotaDebito_V1.0.0.xml", "1.0.0", "NOTA DE D\xc9BITO", "Nota de débito"},
		{"ComprobanteRetencion_V2.0.0.xml", "2.0.0", "COMPROBANTE DE RETENCI\xd3N", "Comprobante de retención"},
		{"GuiaRemision_V1.1.0.xml", "1.1.0", "GU\xcdA DE REMISI\xd3N", "Guía de remisión"},
		{"LiquidacionCompra_V1.1.0.xml", "1.1.0", "LIQUIDACI\xd3N DE COMPRA", "Liquidación de compra"},
	}
	for _, c := range casos {
		t.Run(c.tipo, func(t *testing.T) {
			doc := ejemploOficial(t, c.archivo, c.version)
			pdf, err := ride.Generar(doc, nil)
			if err != nil {
				t.Fatal(err)
			}
			texto, paginas := contenido(t, pdf)
			if paginas < 1 || !strings.Contains(texto, "("+c.titulo) {
				t.Errorf("páginas = %d; falta el título %q", paginas, c.titulo)
			}
			for _, quiero := range []string{"(razonSocial0)", "(0000000000001)", "(CLAVE DE ACCESO)", "(PRUEBAS)", "(000-000-000000000)"} {
				if !strings.Contains(texto, quiero) {
					t.Errorf("falta %s", quiero)
				}
			}
			r, err := ride.Resumir(doc)
			if err != nil {
				t.Fatal(err)
			}
			if r.Tipo != c.tipo || r.Numero != "000-000-000000000" || r.Emisor != "razonSocial0" || r.Receptor == "" || len(r.ClaveAcceso) != 49 {
				t.Errorf("resumen = %+v", r)
			}
		})
	}
}

func facturaDePrueba(t *testing.T, lineas int) []byte {
	t.Helper()
	f := ejemplo.Factura(comprobante.Hoy(), "000000007")
	f.Detalles = nil
	for i := 0; i < lineas; i++ {
		f.Detalles = append(f.Detalles, factura.LineaIVA(fmt.Sprintf("P-%03d", i), "Producto de prueba con ñ y (paréntesis)",
			decimal.Entero(1), decimal.MustParse("1.00"), comprobante.IVA15))
	}
	f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}}
	f.InfoFactura.Pagos[0].Total = decimal.Cero
	f.Calcular()
	if _, err := f.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	doc, err := f.XML()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestFacturaTotalesYPaginacion(t *testing.T) {
	texto, paginas := contenido(t, must(ride.Generar(facturaDePrueba(t, 2), nil)))
	if paginas != 1 {
		t.Errorf("páginas = %d", paginas)
	}
	for _, quiero := range []string{"(SUBTOTAL 15%)", "(IVA 15%)", "(VALOR TOTAL)", "(2.30)",
		"(Producto de prueba", "par\xe9ntesis", "01 - SIN UTILIZACI"} {
		if !strings.Contains(texto, quiero) {
			t.Errorf("falta %q", quiero)
		}
	}
	// 150 líneas no caben en una página: la tabla continúa y repite la
	// cabecera.
	texto, paginas = contenido(t, must(ride.Generar(facturaDePrueba(t, 150), nil)))
	if paginas < 3 {
		t.Errorf("páginas = %d", paginas)
	}
	if n := strings.Count(texto, "(Precio Total)"); n != paginas {
		t.Errorf("la cabecera de la tabla aparece %d veces en %d páginas", n, paginas)
	}
	if !strings.Contains(texto, "(P-149)") || !strings.Contains(texto, "(VALOR TOTAL)") {
		t.Error("falta la última línea o los totales")
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestAutorizacion(t *testing.T) {
	doc := facturaDePrueba(t, 1)
	envolver := func(estado string) []byte {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?><autorizacion><estado>` + estado +
			`</estado><numeroAutorizacion>1234567890</numeroAutorizacion><fechaAutorizacion>2026-11-14T23:56:59-05:00</fechaAutorizacion>` +
			`<ambiente>PRUEBAS</ambiente><comprobante><![CDATA[` + string(doc) + `]]></comprobante></autorizacion>`)
	}
	texto, _ := contenido(t, must(ride.Generar(envolver("AUTORIZADO"), nil)))
	if !strings.Contains(texto, "(14/11/2026 23:56:59)") || !strings.Contains(texto, "(1234567890)") {
		t.Error("faltan el número o la fecha de autorización")
	}
	if _, err := ride.Generar(envolver("NO AUTORIZADO"), nil); !errors.Is(err, ride.ErrNoAutorizado) {
		t.Errorf("esperaba ErrNoAutorizado, obtuve %v", err)
	}
	if _, err := ride.Resumir(envolver("EN PROCESO")); !errors.Is(err, ride.ErrNoAutorizado) {
		t.Errorf("esperaba ErrNoAutorizado, obtuve %v", err)
	}
	// Sin envoltura, el número de autorización es la clave de acceso y la
	// fecha solo aparece si se indica.
	texto, _ = contenido(t, must(ride.Generar(doc, nil)))
	if strings.Contains(texto, "FECHA Y HORA DE AUTORIZACI") {
		t.Error("no debe figurar una fecha de autorización que no se conoce")
	}
	texto, _ = contenido(t, must(ride.Generar(doc, &ride.Opciones{FechaAutorizacion: "15/11/2026 10:00:00", SinCodigoBarras: true})))
	if !strings.Contains(texto, "(15/11/2026 10:00:00)") {
		t.Error("falta la fecha indicada en las opciones")
	}
}

func TestLogoYErrores(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		img.Set(x, 10, color.RGBA{0, 80, 160, 255})
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	doc := facturaDePrueba(t, 1)
	pdf, err := ride.Generar(doc, &ride.Opciones{Logo: buf.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Subtype /Image /Width 40 /Height 20")) {
		t.Error("el logotipo no está en el PDF")
	}
	// Un JPEG se incrusta tal cual, sin recomprimir.
	var jb bytes.Buffer
	jpeg.Encode(&jb, img, nil)
	pdf, err = ride.Generar(doc, &ride.Opciones{Logo: jb.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Filter /DCTDecode")) || !bytes.Contains(pdf, jb.Bytes()) {
		t.Error("el JPEG no se incrustó tal cual")
	}
	// Un JPEG enorme se reduce y sigue siendo JPEG.
	jb.Reset()
	jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 2000, 2000)), nil)
	pdf, err = ride.Generar(doc, &ride.Opciones{Logo: jb.Bytes()})
	if err != nil || !bytes.Contains(pdf, []byte("/Width 900 /Height 900")) || !bytes.Contains(pdf, []byte("/Filter /DCTDecode")) {
		t.Errorf("el JPEG grande no se redujo: %v", err)
	}
	// Un PNG enorme se reduce.
	grande := image.NewRGBA(image.Rect(0, 0, 3000, 1500))
	buf.Reset()
	png.Encode(&buf, grande)
	pdf, err = ride.Generar(doc, &ride.Opciones{Logo: buf.Bytes()})
	if err != nil || !bytes.Contains(pdf, []byte("/Width 900 /Height 450")) {
		t.Errorf("el PNG grande no se redujo: %v", err)
	}
	// Sin logotipo, el espacio queda reservado con la leyenda del SRI,
	// salvo que se pida en blanco.
	texto, _ := contenido(t, must(ride.Generar(doc, nil)))
	if !strings.Contains(texto, "(NO TIENE LOGO)") {
		t.Error("falta la leyenda NO TIENE LOGO")
	}
	texto, _ = contenido(t, must(ride.Generar(doc, &ride.Opciones{SinLeyendaLogo: true})))
	if strings.Contains(texto, "NO TIENE LOGO") {
		t.Error("no debe figurar la leyenda")
	}
	if _, err := ride.Generar(doc, &ride.Opciones{Logo: []byte("no es una imagen")}); !errors.Is(err, ride.ErrLogo) {
		t.Errorf("esperaba ErrLogo, obtuve %v", err)
	}
	if _, err := ride.Generar([]byte("<otra/>"), nil); err == nil {
		t.Error("esperaba error con un documento desconocido")
	}
	if _, err := ride.Generar([]byte("no es xml"), nil); err == nil {
		t.Error("esperaba error con un documento ilegible")
	}
}

// Al repartir el espacio de un comprobante corto no debe desbordarse a una
// segunda página, y el número de páginas solo puede crecer con las líneas.
func TestRepartoDelEspacio(t *testing.T) {
	previas := 1
	for n := 1; n <= 60; n += 3 {
		_, paginas := contenido(t, must(ride.Generar(facturaDePrueba(t, n), nil)))
		if paginas < previas || (n <= 4 && paginas != 1) {
			t.Errorf("%d líneas: %d páginas (antes %d)", n, paginas, previas)
		}
		previas = paginas
	}
	pdf := must(ride.Generar(facturaDePrueba(t, 2), nil))
	if !bytes.Contains(pdf, []byte("/Type /Page /Parent")) {
		t.Fatal("sin páginas")
	}
	if texto, _ := contenido(t, pdf); !strings.Contains(texto, " c h S") {
		t.Error("las cajas y tablas deben tener el contorno redondeado")
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/guiaremision"
	"github.com/OsirisRoman/sri-ecuador-go/liquidacion"
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
	"github.com/OsirisRoman/sri-ecuador-go/ride"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

// comillas deshace el escape de comillas de encoding/xml: al firmar, el
// documento se reescribe y las comillas del texto quedan literales, que es
// una forma equivalente.
var comillas = strings.NewReplacer("&#34;", `"`, "&#39;", "'")

// regresion vuelve a pasar por el SDK actual todos los comprobantes que el
// SRI autorizó y que se conservan en dir: XSD, verificación de la firma,
// reglas locales (con la fecha de emisión como fecha de referencia), ida y
// vuelta del modelo y RIDE. No se comunica con el SRI. Sirve para comprobar
// que un cambio no invalida lo que ya se demostró que el SRI acepta.
func regresion(dir string) int {
	rutas, _ := filepath.Glob(filepath.Join(dir, "*.autorizado.xml"))
	mas, _ := filepath.Glob(filepath.Join(dir, "qa-*", "*.autorizado.xml"))
	rutas = append(rutas, mas...)
	fallos, porTipo := 0, map[esquema.Tipo]int{}
	for _, ruta := range rutas {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			panic(err)
		}
		s := string(datos)
		a, b := strings.Index(s, "<![CDATA["), strings.LastIndex(s, "]]>")
		if a < 0 || b < a {
			continue
		}
		doc := []byte(s[a+9 : b])
		nombre := filepath.Base(ruta)
		falla := func(etapa string, err error) {
			fallos++
			fmt.Printf("FALLA %s: %s: %s\n", nombre, etapa, sanear(err.Error()))
		}
		tipo, version, err := esquema.Identificar(doc)
		if err != nil {
			falla("identificar", err)
			continue
		}
		porTipo[tipo]++
		if err := esquema.Validar(doc); err != nil {
			falla("XSD", err)
		}
		if _, err := xades.Verificar(doc, nil); err != nil {
			falla("firma", err)
		}
		var c sri.Comprobante
		var hs comprobante.Hallazgos
		switch tipo {
		case esquema.Factura:
			f, err := factura.Parse(doc)
			if err == nil {
				c, hs = f, f.ValidarCon(factura.OpcionesValidacion{Hoy: f.InfoFactura.FechaEmision})
			}
		case esquema.NotaCredito:
			n, err := notacredito.Parse(doc)
			if err == nil {
				c, hs = n, n.ValidarCon(notacredito.OpcionesValidacion{Hoy: n.InfoNotaCredito.FechaEmision})
			}
		case esquema.NotaDebito:
			n, err := notadebito.Parse(doc)
			if err == nil {
				c, hs = n, n.ValidarCon(notadebito.OpcionesValidacion{Hoy: n.InfoNotaDebito.FechaEmision})
			}
		case esquema.ComprobanteRetencion:
			if version == retencion.VersionV1 {
				r, err := retencion.ParseV1(doc)
				if err == nil {
					c, hs = r, r.ValidarCon(retencion.OpcionesValidacion{Hoy: r.InfoCompRetencion.FechaEmision})
				}
			} else {
				r, err := retencion.Parse(doc)
				if err == nil {
					c, hs = r, r.ValidarCon(retencion.OpcionesValidacion{Hoy: r.InfoCompRetencion.FechaEmision})
				}
			}
		case esquema.GuiaRemision:
			g, err := guiaremision.Parse(doc)
			if err == nil {
				c, hs = g, g.ValidarCon(guiaremision.OpcionesValidacion{Hoy: g.FechaEmision()})
			}
		case esquema.LiquidacionCompra:
			l, err := liquidacion.Parse(doc)
			if err == nil {
				c, hs = l, l.ValidarCon(liquidacion.OpcionesValidacion{Hoy: l.InfoLiquidacionCompra.FechaEmision})
			}
		}
		if c == nil {
			falla("lectura", fmt.Errorf("no se pudo leer como %s", tipo))
			continue
		}
		if err := hs.Err(); err != nil {
			falla("reglas", err)
		}
		// El modelo debe reproducir exactamente lo que se firmó: el XML sin
		// firma tiene que ser un prefijo del documento autorizado.
		sinFirma, err := c.XML()
		if err != nil {
			falla("serializar", err)
		} else if cuerpo := strings.TrimSuffix(comillas.Replace(string(sinFirma)), "</"+string(tipo)+">"); !strings.HasPrefix(string(doc), cuerpo) {
			falla("ida y vuelta", fmt.Errorf("el modelo no reproduce el documento autorizado"))
		}
		if _, err := ride.Generar(datos, nil); err != nil {
			falla("RIDE", err)
		}
	}
	fmt.Printf("comprobantes autorizados revisados: %d; fallos: %d\n", len(rutas), fallos)
	for t, n := range porTipo {
		fmt.Printf("  %s: %d\n", t, n)
	}
	return fallos
}

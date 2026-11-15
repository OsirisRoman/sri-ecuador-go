package ws_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

func claveLote(t *testing.T, secuencial int) string {
	t.Helper()
	c, err := clave.Generar(clave.Componentes{
		FechaEmision: comprobante.Hoy().Time(), TipoComprobante: comprobante.CodFactura, RUC: ejemplo.RUC, Ambiente: "1",
		Establecimiento: "001", PuntoEmision: "999", Secuencial: fmt.Sprintf("%09d", secuencial), CodigoNumerico: "12345678",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func firmadas(t *testing.T, n int) [][]byte {
	t.Helper()
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	var docs [][]byte
	for i := 1; i <= n; i++ {
		f := ejemplo.Factura(comprobante.Hoy(), fmt.Sprintf("%09d", i))
		doc, err := f.XML()
		if err != nil {
			t.Fatal(err)
		}
		firmado, err := xades.Firmar(doc, m.Certificado, nil)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, firmado)
	}
	return docs
}

func TestLoteXML(t *testing.T) {
	docs := [][]byte{[]byte("<factura>a]]>b</factura>"), []byte("<factura/>")}
	l := ws.Lote{ClaveAcceso: claveLote(t, 1), RUC: ejemplo.RUC, Comprobantes: docs}
	doc, err := l.XML()
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	if !strings.HasPrefix(s, `<?xml version="1.0" encoding="UTF-8"?><lote version="1.0.0"><claveAcceso>`) ||
		strings.Count(s, "<comprobante><![CDATA[") != 2 || !strings.Contains(s, "a]]]]><![CDATA[>b") {
		t.Errorf("lote = %s", s)
	}
	for nombre, malo := range map[string]ws.Lote{
		"sin comprobantes":  {ClaveAcceso: l.ClaveAcceso, RUC: l.RUC},
		"clave inválida":    {ClaveAcceso: "123", RUC: l.RUC, Comprobantes: docs},
		"RUC corto":         {ClaveAcceso: l.ClaveAcceso, RUC: "123", Comprobantes: docs},
		"comprobante vacío": {ClaveAcceso: l.ClaveAcceso, RUC: l.RUC, Comprobantes: [][]byte{nil}},
		"más de 50":         {ClaveAcceso: l.ClaveAcceso, RUC: l.RUC, Comprobantes: make([][]byte, 51)},
		"más de 500 kB":     {ClaveAcceso: l.ClaveAcceso, RUC: l.RUC, Comprobantes: [][]byte{[]byte(strings.Repeat("x", ws.TamanoMaximoLote))}},
	} {
		if _, err := malo.XML(); !errors.Is(err, ws.ErrLote) {
			t.Errorf("%s: esperaba ErrLote, obtuve %v", nombre, err)
		}
	}
}

// Reproduce lo observado en el ambiente de pruebas del SRI: el lote se
// recibe entero y cada comprobante se autoriza o no por separado.
func TestLoteContraElSimulador(t *testing.T) {
	srv := sritest.Nuevo()
	defer srv.Cerrar()
	c := ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints()))
	ctx := context.Background()

	docs := firmadas(t, 3)
	docs[1] = []byte(strings.Replace(string(docs[1]), "<razonSocial>", "<razonSocial>X", 1))           // firma rota
	docs[2] = []byte(strings.Replace(string(docs[2]), "<infoFactura>", "<noExiste/><infoFactura>", 1)) // XSD roto
	l := ws.Lote{ClaveAcceso: claveLote(t, 1), RUC: ejemplo.RUC, Comprobantes: docs}

	rec, err := c.ValidarLote(ctx, l)
	if err != nil || !rec.Recibida() {
		t.Fatalf("recepción = %+v, %v", rec, err)
	}
	r, err := c.AutorizacionLote(ctx, l.ClaveAcceso)
	if err != nil {
		t.Fatal(err)
	}
	if r.ClaveAccesoLoteConsultada != l.ClaveAcceso || r.NumeroComprobantesLote != "3" || len(r.Autorizaciones) != 3 {
		t.Fatalf("respuesta = %+v", r)
	}
	quiero := []struct {
		autorizado bool
		codigo     string
	}{{true, ""}, {false, "39"}, {false, "35"}}
	for i, a := range r.Autorizaciones {
		if a.Autorizado() != quiero[i].autorizado {
			t.Errorf("comprobante %d: estado %q", i, a.Estado)
		}
		if quiero[i].codigo != "" && (len(a.Mensajes) == 0 || a.Mensajes[0].Identificador != quiero[i].codigo) {
			t.Errorf("comprobante %d: mensajes %v; esperaba el código %s", i, a.Mensajes, quiero[i].codigo)
		}
	}
	// El comprobante autorizado dentro del lote se consulta también por su
	// propia clave.
	if _, ok := srv.Registrado(clavePrimera(t, docs[0])); !ok {
		t.Error("el primer comprobante no quedó registrado")
	}
	// La misma clave de lote no puede reutilizarse.
	if rec, err := c.ValidarLote(ctx, l); err != nil || rec.Recibida() {
		t.Errorf("un lote repetido debe devolverse: %+v, %v", rec, err)
	}
	// Un lote desconocido no tiene resultados.
	if r, err := c.AutorizacionLote(ctx, claveLote(t, 2)); err != nil || len(r.Autorizaciones) != 0 {
		t.Errorf("lote desconocido = %+v, %v", r, err)
	}
	if _, err := c.AutorizacionLote(ctx, "123"); err == nil {
		t.Error("esperaba error con una clave inválida")
	}
}

func clavePrimera(t *testing.T, doc []byte) string {
	t.Helper()
	s := string(doc)
	a, b := strings.Index(s, "<claveAcceso>"), strings.Index(s, "</claveAcceso>")
	if a < 0 || b < a {
		t.Fatal("comprobante sin clave de acceso")
	}
	return s[a+13 : b]
}

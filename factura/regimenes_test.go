package factura_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// recalcular rehace los totales tras cambiar la factura base.
func recalcular(f *factura.Factura) {
	for i := range f.InfoFactura.Pagos {
		f.InfoFactura.Pagos[i].Total = d("0")
	}
	f.InfoFactura.Pagos = f.InfoFactura.Pagos[:1]
	f.Calcular()
}

func exportacion(t *testing.T) *factura.Factura {
	f := base(t)
	i := &f.InfoFactura
	i.ComercioExterior = factura.ComercioExteriorExportador
	i.IncoTermFactura, i.LugarIncoTerm = "CIF", "GUAYAQUIL"
	i.PaisOrigen, i.PaisDestino, i.PaisAdquisicion = "593", "331", "331"
	i.PuertoEmbarque, i.PuertoDestino = "GUAYAQUIL", "SHANGHAI"
	i.IncoTermTotalSinImpuestos = "FOB"
	flete, seguro := d("100.00"), d("20.50")
	i.FleteInternacional, i.SeguroInternacional = &flete, &seguro
	recalcular(f)
	return f
}

func sinHallazgosYXSD(t *testing.T, f *factura.Factura) {
	t.Helper()
	if hs := validar(f); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, err := f.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
}

func TestExportacion(t *testing.T) {
	f := exportacion(t)
	sinFlete := base(t).InfoFactura.ImporteTotal
	if got := f.InfoFactura.ImporteTotal; !got.Igual(sinFlete.Sumar(d("120.50"))) {
		t.Errorf("importe = %s; el flete y el seguro deben sumarse a %s", got, sinFlete)
	}
	sinHallazgosYXSD(t, f)

	casos := []struct {
		nombre string
		mutar  func(*factura.Factura)
		regla  string
	}{
		{"valor de comercioExterior", func(f *factura.Factura) { f.InfoFactura.ComercioExterior = "IMPORTADOR" }, "catalogo"},
		{"sin incoterm", func(f *factura.Factura) { f.InfoFactura.IncoTermFactura = "" }, "requerido"},
		{"sin puerto de destino", func(f *factura.Factura) { f.InfoFactura.PuertoDestino = "" }, "requerido"},
		{"sin incoterm del total", func(f *factura.Factura) { f.InfoFactura.IncoTermTotalSinImpuestos = "" }, "requerido"},
		{"país con letras", func(f *factura.Factura) { f.InfoFactura.PaisDestino = "CN" }, "formato"},
		{"campos de exportación sin comercioExterior", func(f *factura.Factura) { f.InfoFactura.ComercioExterior = "" }, "exportacion.comercio-exterior"},
	}
	// El SRI rechazó «156» (China en ISO 3166): su tabla usa 331.
	f = exportacion(t)
	f.InfoFactura.PaisDestino = "156"
	if hs := validar(f); !hs.Tiene("pais.catalogo") || hs.Err() != nil {
		t.Errorf("esperaba la advertencia pais.catalogo: %v", hs)
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := exportacion(t)
			c.mutar(f)
			if hs := validar(f); !hs.Tiene(c.regla) || hs.Err() == nil {
				t.Errorf("no se detectó %q: %v", c.regla, hs)
			}
		})
	}
}

func reembolso() factura.ReembolsoDetalle {
	return factura.ReembolsoDetalle{
		TipoIdentificacionProveedorReembolso: comprobante.IDRuc,
		IdentificacionProveedorReembolso:     "1790011674001",
		CodPaisPagoProveedorReembolso:        "593",
		TipoProveedorReembolso:               "02",
		CodDocReembolso:                      "01",
		EstabDocReembolso:                    "001", PtoEmiDocReembolso: "002", SecuencialDocReembolso: "000000123",
		FechaEmisionDocReembolso:   comprobante.NuevaFecha(2026, 11, 10),
		NumeroAutorizacionDocReemb: "1011202601179001167400110010020000001231234567818",
		DetalleImpuestos: []factura.ImpuestoReembolso{{
			Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15, Tarifa: d("15"),
			BaseImponibleReembolso: d("50.00"), ImpuestoReembolso: d("7.50"),
		}},
	}
}

func TestReembolsos(t *testing.T) {
	f := base(t)
	f.Reembolsos = []factura.ReembolsoDetalle{reembolso(), reembolso()}
	if hs := validar(f); !hs.Tiene("reembolso.codigo") || !hs.Tiene("reembolso.totales") {
		t.Errorf("sin calcular debe fallar: %v", hs)
	}
	recalcular(f)
	i := f.InfoFactura
	if i.CodDocReembolso != "41" || i.TotalBaseImponibleReembolso.String() != "100.00" || i.TotalImpuestoReembolso.String() != "15.00" || i.TotalComprobantesReembolso.String() != "115.00" {
		t.Errorf("totales de reembolso = %+v", i)
	}
	sinHallazgosYXSD(t, f)

	casos := []struct {
		nombre string
		mutar  func(*factura.Factura)
		regla  string
	}{
		{"total alterado", func(f *factura.Factura) { f.InfoFactura.TotalImpuestoReembolso = nil }, "reembolso.totales"},
		{"código 41 sin reembolsos", func(f *factura.Factura) { f.Reembolsos = nil }, "requerido"},
		{"autorización", func(f *factura.Factura) { f.Reembolsos[0].NumeroAutorizacionDocReemb = "123" }, "formato"},
		{"secuencial", func(f *factura.Factura) { f.Reembolsos[1].SecuencialDocReembolso = "123" }, "formato"},
		{"sin impuestos", func(f *factura.Factura) { f.Reembolsos[0].DetalleImpuestos = nil }, "requerido"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := base(t)
			f.Reembolsos = []factura.ReembolsoDetalle{reembolso(), reembolso()}
			recalcular(f)
			c.mutar(f)
			if hs := validar(f); !hs.Tiene(c.regla) || hs.Err() == nil {
				t.Errorf("no se detectó %q: %v", c.regla, hs)
			}
		})
	}
}

func TestPlacaCombustiblesYTransporte(t *testing.T) {
	avisa := func(f *factura.Factura, regla string) {
		t.Helper()
		if hs := validar(f); !hs.Tiene(regla) || hs.Err() != nil {
			t.Errorf("esperaba solo la advertencia %q: %v", regla, hs)
		}
	}
	f := base(t)
	f.Detalles[0].CodigoPrincipal, f.Detalles[0].Descripcion = "0101", "Extra"
	avisa(f, "combustible.placa")
	f.InfoFactura.Placa = "PCM4567"
	sinHallazgosYXSD(t, f)
	f.InfoFactura.Placa = "PCM-456"
	avisa(f, "placa.formato")

	// Un código interno que coincide con la Tabla 30 pero con otra
	// descripción no es un combustible.
	f = base(t)
	f.Detalles[0].CodigoPrincipal = "0101"
	sinHallazgosYXSD(t, f)

	f = base(t)
	f.Detalles[0].CodigoAuxiliar = factura.CodAuxTransporteOperadora
	avisa(f, "transporte.placa")
	f.Detalles[0].CodigoAuxiliar = factura.CodAuxTransporteSocio
	sinHallazgosYXSD(t, f)
}

func TestNegociable(t *testing.T) {
	f := base(t)
	f.TipoNegociable = &factura.TipoNegociable{Correo: "cobros@example.com"}
	f.InfoFactura.DireccionComprador = ""
	f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoEndosoTitulos}}
	f.Calcular()
	hs := validar(f)
	if !hs.Tiene("negociable.direccion") || !hs.Tiene("negociable.plazo") {
		t.Errorf("faltan requisitos del Anexo 11: %v", hs)
	}
	plazo := d("30")
	f.InfoFactura.DireccionComprador = "Salinas y Santiago"
	f.InfoFactura.Pagos[0].Plazo, f.InfoFactura.Pagos[0].UnidadTiempo = &plazo, "dias"
	sinHallazgosYXSD(t, f)
	f.TipoNegociable.Correo = "sin arroba"
	if hs := validar(f); !hs.Tiene("formato") {
		t.Errorf("correo inválido: %v", hs)
	}
}

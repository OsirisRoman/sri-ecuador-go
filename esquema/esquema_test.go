package esquema

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Todos los esquemas incrustados deben cargar, con y sin ajustes.
func TestCarganTodosLosEsquemas(t *testing.T) {
	for k, archivo := range catalogo {
		for _, estricto := range []bool{true, false} {
			if _, err := cargar(archivo, estricto); err != nil {
				t.Errorf("%s %s (estricto=%t): %v", k.tipo, k.version, estricto, err)
			}
		}
	}
}

// Los XSD deben ser exactamente los publicados por el SRI: PROCEDENCIA.md
// registra sus SHA-256 y esta prueba detecta cualquier cambio.
func TestProcedencia(t *testing.T) {
	reg, err := os.ReadFile("xsd/PROCEDENCIA.md")
	if err != nil {
		t.Fatal(err)
	}
	archivos, _ := filepath.Glob("xsd/*.xsd")
	if len(archivos) != 17 {
		t.Errorf("se esperaban 17 XSD, hay %d", len(archivos))
	}
	for _, f := range archivos {
		b, _ := os.ReadFile(f)
		s := sha256.Sum256(b)
		linea := hex.EncodeToString(s[:]) + "  " + filepath.Base(f)
		if !strings.Contains(string(reg), linea) {
			t.Errorf("%s no coincide con PROCEDENCIA.md (sha256 %s)", f, hex.EncodeToString(s[:]))
		}
	}
}

func TestIdentificarYErrores(t *testing.T) {
	tipo, v, err := Identificar([]byte(`<notaCredito id="comprobante" version="1.1.0"/>`))
	if err != nil || tipo != NotaCredito || v != "1.1.0" {
		t.Errorf("Identificar = %q %q %v", tipo, v, err)
	}
	if err := Validar([]byte(`<pedido version="1.0.0"/>`)); !errors.Is(err, ErrTipoDesconocido) {
		t.Errorf("tipo: %v", err)
	}
	if err := Validar([]byte(`<factura id="comprobante" version="9.9.9"/>`)); !errors.Is(err, ErrVersionDesconocida) {
		t.Errorf("versión: %v", err)
	}
	err = Validar([]byte(`<factura id="comprobante" version="1.1.0"><infoTributaria/></factura>`))
	var ev *ErrorValidacion
	if !errors.As(err, &ev) || len(ev.Problemas) == 0 {
		t.Fatalf("esperaba ErrorValidacion, obtuve %v", err)
	}
	if !strings.Contains(ev.Problemas[0].Ruta, "/factura") {
		t.Errorf("ruta = %q", ev.Problemas[0].Ruta)
	}
}

// Los XML de ejemplo que el SRI distribuye junto a los XSD son plantillas
// con valores de relleno ("razonSocial0", version="version0"), no documentos
// válidos; se registra qué opina el validador de cada uno para detectar
// cambios de comportamiento.
func TestEjemplosOficiales(t *testing.T) {
	ejemplos, _ := filepath.Glob("testdata/*.xml")
	if len(ejemplos) == 0 {
		t.Skip("sin ejemplos")
	}
	for _, f := range ejemplos {
		doc, _ := os.ReadFile(f)
		base := filepath.Base(f)
		// La versión real se toma del nombre del archivo.
		i := strings.Index(base, "_V")
		version := strings.TrimSuffix(base[i+2:], ".xml")
		doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="`+version+`"`, 1))
		err := Validar(doc)
		var ev *ErrorValidacion
		switch {
		case err == nil:
			t.Logf("%s: válido", base)
		case errors.As(err, &ev):
			t.Logf("%s: %d problema(s); primero: %s", base, len(ev.Problemas), ev.Problemas[0])
		default:
			t.Errorf("%s: %v", base, err)
		}
	}
}

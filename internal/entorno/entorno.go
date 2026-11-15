// Package entorno lee la configuración privada (certificado y datos del
// emisor) de variables de entorno o de un archivo .env, para las
// herramientas de línea de comandos del repositorio. Ver .env.example.
package entorno

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
)

// CargarEnv lee pares CLAVE=valor de un archivo sin sobrescribir variables
// ya definidas en el entorno. Si el archivo no existe no hace nada.
func CargarEnv(ruta string) {
	f, err := os.Open(ruta)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		linea := strings.TrimSpace(sc.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		k, v, ok := strings.Cut(linea, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if _, existe := os.LookupEnv(k); !existe {
			os.Setenv(k, v)
		}
	}
}

// Valor devuelve una variable o su valor por defecto.
func Valor(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}

// Certificado carga el .p12 indicado por SRI_P12_PATH con la contraseña de
// SRI_P12_PASSWORD o del archivo SRI_P12_PASSWORD_FILE.
func Certificado() (*certificado.Certificado, error) {
	ruta := os.Getenv("SRI_P12_PATH")
	if ruta == "" {
		return nil, errors.New("defina SRI_P12_PATH con la ruta del archivo .p12")
	}
	clave := os.Getenv("SRI_P12_PASSWORD")
	if f := os.Getenv("SRI_P12_PASSWORD_FILE"); clave == "" && f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer SRI_P12_PASSWORD_FILE: %w", err)
		}
		clave = strings.TrimRight(string(b), "\r\n")
	}
	if clave == "" {
		return nil, errors.New("defina SRI_P12_PASSWORD o SRI_P12_PASSWORD_FILE")
	}
	if info, err := os.Stat(ruta); err == nil && info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "aviso: %s es legible por otros usuarios; se recomienda chmod 600\n", ruta)
	}
	return certificado.CargarArchivo(ruta, clave)
}

// Emisor construye el bloque infoTributaria del emisor configurado, en
// ambiente de pruebas, para el secuencial dado.
func Emisor(codDoc string, secuencial int) (comprobante.InfoTributaria, error) {
	for _, v := range []string{"SRI_EMISOR_RUC", "SRI_EMISOR_RAZON_SOCIAL", "SRI_EMISOR_DIR_MATRIZ"} {
		if os.Getenv(v) == "" {
			return comprobante.InfoTributaria{}, fmt.Errorf("falta la variable %s (ver .env.example)", v)
		}
	}
	if secuencial < 1 || secuencial > 999999999 {
		return comprobante.InfoTributaria{}, errors.New("secuencial fuera de rango (1-999999999)")
	}
	return comprobante.InfoTributaria{
		Ambiente: comprobante.Pruebas, TipoEmision: comprobante.EmisionNormal,
		RazonSocial:        os.Getenv("SRI_EMISOR_RAZON_SOCIAL"),
		NombreComercial:    os.Getenv("SRI_EMISOR_NOMBRE_COMERCIAL"),
		RUC:                os.Getenv("SRI_EMISOR_RUC"),
		CodDoc:             codDoc,
		Estab:              Valor("SRI_EMISOR_ESTAB", "001"),
		PtoEmi:             Valor("SRI_EMISOR_PTO_EMI", "001"),
		Secuencial:         fmt.Sprintf("%09d", secuencial),
		DirMatriz:          os.Getenv("SRI_EMISOR_DIR_MATRIZ"),
		AgenteRetencion:    os.Getenv("SRI_EMISOR_AGENTE_RETENCION"),
		ContribuyenteRimpe: os.Getenv("SRI_EMISOR_CONTRIBUYENTE_RIMPE"),
	}, nil
}

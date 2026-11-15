package comprobante

import (
	"errors"
	"fmt"
)

// Errores de identificación.
var (
	ErrRUC    = errors.New("RUC inválido")
	ErrCedula = errors.New("cédula inválida")
)

// provinciaValida comprueba los dos primeros dígitos: 01-24 son provincias y
// 30 se asigna a ecuatorianos registrados en el exterior.
func provinciaValida(s string) bool {
	p := int(s[0]-'0')*10 + int(s[1]-'0')
	return (p >= 1 && p <= 24) || p == 30
}

// ValidarRUC comprueba la forma de un RUC: 13 dígitos, código de provincia
// válido y establecimiento distinto de 000.
//
// No se exige el dígito verificador. El SRI asigna desde hace años RUC de
// sociedades y de personas extranjeras que no satisfacen el módulo 11
// clásico, de modo que exigirlo rechazaría contribuyentes reales; la
// existencia del RUC la verifica el SRI al autorizar (errores 2 y 46).
func ValidarRUC(ruc string) error {
	if len(ruc) != 13 || !SoloDigitos(ruc) {
		return fmt.Errorf("%w: debe tener 13 dígitos", ErrRUC)
	}
	if !provinciaValida(ruc) {
		return fmt.Errorf("%w: código de provincia %q", ErrRUC, ruc[:2])
	}
	if ruc[10:] == "000" {
		return fmt.Errorf("%w: el establecimiento no puede ser 000", ErrRUC)
	}
	return nil
}

// ValidarCedula comprueba una cédula de identidad ecuatoriana: 10 dígitos,
// provincia válida, tercer dígito menor que 6 y dígito verificador
// (módulo 10).
func ValidarCedula(cedula string) error {
	if len(cedula) != 10 || !SoloDigitos(cedula) {
		return fmt.Errorf("%w: debe tener 10 dígitos", ErrCedula)
	}
	if !provinciaValida(cedula) {
		return fmt.Errorf("%w: código de provincia %q", ErrCedula, cedula[:2])
	}
	if cedula[2] > '5' {
		return fmt.Errorf("%w: tercer dígito %c", ErrCedula, cedula[2])
	}
	suma := 0
	for i := 0; i < 9; i++ {
		d := int(cedula[i] - '0')
		if i%2 == 0 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		suma += d
	}
	if dv := (10 - suma%10) % 10; dv != int(cedula[9]-'0') {
		return fmt.Errorf("%w: dígito verificador incorrecto", ErrCedula)
	}
	return nil
}

// Identificacion valida la pareja tipo/número de identificación de un
// comprador, sujeto retenido o destinatario (Tabla 6). permiteConsumidorFinal
// es false en los comprobantes que exigen identificar al receptor.
//
// Las cédulas con dígito verificador incorrecto se reportan como
// advertencia, igual que hace el SRI (advertencia 62), porque no impiden la
// autorización.
func (v *Validador) Identificacion(campoTipo, campoNumero, tipo, numero string, permiteConsumidorFinal bool) {
	switch tipo {
	case IDRuc:
		if err := ValidarRUC(numero); err != nil {
			v.Errorf(campoNumero, "ruc.formato", "%v", err)
		}
	case IDCedula:
		if len(numero) != 10 || !SoloDigitos(numero) {
			v.Errorf(campoNumero, "cedula.formato", "la cédula debe tener 10 dígitos")
		} else if err := ValidarCedula(numero); err != nil {
			v.Advertirf(campoNumero, "cedula.verificador", "%v (el SRI lo reporta como advertencia 62)", err)
		}
	case IDPasaporte, IDExterior:
		v.Texto(campoNumero, numero, true, 20)
	case IDConsumidorFinal:
		if !permiteConsumidorFinal {
			v.Errorf(campoTipo, "consumidor-final.no-permitido", "este comprobante exige identificar al receptor (Tabla 6)")
		} else if numero != IdentificacionConsumidorFinal {
			v.Errorf(campoNumero, "consumidor-final.identificacion", "para consumidor final debe ser %s", IdentificacionConsumidorFinal)
		}
	default:
		v.Errorf(campoTipo, "catalogo", "tipo de identificación %q desconocido (Tabla 6: 04-08)", tipo)
	}
}

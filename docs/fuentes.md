# Fuentes y licencias

## Normativa (define las obligaciones)

| Fuente | Versión | Uso |
|---|---|---|
| *Ficha técnica de comprobantes electrónicos esquema offline*, SRI | **2.34** (actualizada julio 2026, 142 págs.) | Especificación de referencia. Las citas del código ("sección 5.2", "Anexo 14") remiten a ella |
| Esquemas XSD y XML de ejemplo, SRI (<https://www.sri.gob.ec/facturacion-electronica>) | descargados el 2026-11-14 | Incrustados sin modificar en `esquema/xsd/`; SHA-256 en `esquema/xsd/PROCEDENCIA.md` |
| WSDL de los servicios (`celcer.sri.gob.ec`) | consultados el 2026-11-14 | Forma de los mensajes SOAP de `ws` |
| ETSI TS 101 903 v1.3.2 (XAdES), W3C XMLDSig, W3C Canonical XML 1.0, RFC 2253 | — | `xades`, `internal/xmldom` |

## Implementaciones estudiadas (ayudan a descubrir cómo cumplir)

Se clonaron y se leyeron su código, pruebas y dependencias. **No se copió código de ninguna**: el SDK es una implementación original. No son dependencias.

| Proyecto | Commit | Fecha | Licencia | Qué se observó |
|---|---|---|---|---|
| [nelsonmarro/go_ec_sri_invoice_signer](https://github.com/nelsonmarro/go_ec_sri_invoice_signer) | `f0ea649` | 2026-11-14 | MIT | Firma con `goxmldsig` + `etree`; SHA-1 por defecto, SHA-256 opcional; autoverificación tras firmar |
| [samuelcampozano/sri-ec](https://github.com/samuelcampozano/sri-ec) | `872dec9` | 2026-08-10 | MIT | Cliente SOAP con sobres fijos; delega la firma en el proyecto anterior |
| [juanqui-art/Facturacion-Electronica](https://github.com/juanqui-art/Facturacion-Electronica) | `3e586d8` | 2026-09-23 | **Sin licencia** | Aplicación completa (API, SQLite, PDF). Solo se estudió su comportamiento |
| [nulldoomer/Open-SRI](https://github.com/nulldoomer/Open-SRI) | `d46ef03` | 2026-10-21 | Apache-2.0 | SDK Java sobre `xades4j` (perfil BES por defecto); modelos de todos los comprobantes; copia de los XSD del SRI |
| [luisgoncalves/xades4j](https://github.com/luisgoncalves/xades4j) | `c27be60` | 2026-09-15 | **LGPL-3.0** | Referencia de XAdES. Por su licencia no se reutiliza código; sirve como verificador externo |

### Candidatos para validación XSD

| Proyecto | Commit | Licencia | Evaluación |
|---|---|---|---|
| [cB-Sigma/xsd](https://github.com/cB-Sigma/xsd) | `6a1eef3` (v1.0.0) | Apache-2.0 | **Adoptado.** Solo biblioteca estándar, sin cgo, ~8,7 k líneas; carga los XSD del SRI y coincide con libxml2 en los 13 ejemplos oficiales |
| [lestrrat-go/helium](https://github.com/lestrrat-go/helium) | `030e98f` | MIT | Descartado: reimplementación completa de libxml2 (~230 k líneas, 3 dependencias) para una sola función |
| [knroy/go-xml](https://github.com/knroy/go-xml) | `4f662ec` | MIT | Descartado: pila XML completa (XPath, XQuery, XSLT, ~245 k líneas) |

## Dependencias de ejecución

| Módulo | Licencia | Para qué |
|---|---|---|
| `github.com/cB-Sigma/xsd` | Apache-2.0 | Validación XSD |
| `software.sslmate.com/src/go-pkcs12` | BSD-3-Clause | Lectura de `.p12` (la biblioteca estándar no lo incluye) |
| `golang.org/x/crypto` | BSD-3-Clause | Dependencia de la anterior |

Todo lo demás (decimal, clave de acceso, árbol XML, C14N, XAdES, SOAP) es código propio sobre la biblioteca estándar.

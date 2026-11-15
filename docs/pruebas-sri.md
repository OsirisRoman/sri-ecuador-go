# Pruebas de interoperabilidad

Este documento registra **resultados reales**, con sus condiciones. Lo que no se ha probado se declara como tal.

## Verificación local independiente

| Qué | Con qué | Resultado |
|---|---|---|
| Firma XAdES-BES RSA-SHA1 (3 referencias) | `xmlsec1` 1.3.6 (OpenSSL), Alpine 3.20 | `Verification status: OK` |
| Firma XAdES-BES RSA-SHA256 | ídem | `Verification status: OK` |
| Validador XSD frente a libxml2 | `xmllint --schema` sobre los 13 XML de ejemplo del SRI | 13/13 veredictos idénticos (12 válidos; `factura_V2.0.0.xml` inválido en ambos) |
| Claves de acceso | Claves impresas en la ficha v2.34 (Anexo 1, Anexo 14, 8.2.3) | Todas verifican; `Generar` reproduce la del Anexo 14 |

`xmlsec1` valida XMLDSig (digests y firma); no evalúa las propiedades XAdES.

## Ambiente de certificación del SRI (`celcer.sri.gob.ec`)

### Ronda 1 — 2026-11-14

Condiciones: emisor persona natural habilitado en pruebas; certificado del Banco Central del Ecuador (RSA 2048, cadena de 4 certificados en el `.p12`); factura v1.1.0 a consumidor final, un detalle de USD 1,00 con IVA 15 %, pago en efectivo; SDK en la rama `feat/flujo-factura`. Los datos del emisor y las claves de acceso se omiten.

| # | Operación | Validación local | Firma verificada | Recepción | Autorización |
|---|---|---|---|---|---|
| 1 | Factura firmada con **RSA-SHA256** | ✅ reglas y XSD | ✅ | `RECIBIDA` | **`AUTORIZADO`** |
| 2 | Reenvío del mismo documento ya autorizado | — | — | `DEVUELTA`, error **43** «CLAVE ACCESO REGISTRADA» | sigue `AUTORIZADO` |
| 3 | Consulta de autorización por clave | — | — | — | `AUTORIZADO`, con fecha |
| 4 | Consulta de validez (`ConsultaComprobante`) | — | — | — | `estadoAutorizacion=AUTORIZADO`, `tipoComprobante=Factura` |

Observaciones:

- El comprobante que el SRI devuelve dentro de la autorización es **idéntico byte a byte** al enviado: la serialización canónica del SDK no es alterada por el SRI.
- El número de autorización es la clave de acceso (ficha 5.9) y el ambiente devuelto es `PRUEBAS`.
- El reenvío se resolvió sin intervención: `Reanudar` interpretó el 43 como «ya recibido» y obtuvo la autorización (decisión D8).
- En la consulta de validez satisfactoria, `estadoConsulta` llega vacío.

### Ronda 2 — 2026-11-14

Mismas condiciones. Ejecutada con `go run ./internal/qa`. Todos los escenarios pasaron antes la validación local (reglas, XSD, firma verificada) y, en los autorizados, el SRI devolvió el comprobante idéntico byte a byte.

| Escenario | Versión | Firma | Recepción | Autorización |
|---|---|---|---|---|
| Comprador con RUC; IVA 15 % con descuento y precio de 6 decimales + IVA 0 %; crédito a 30 días; `detAdicional`; info adicional con `ñ & " < > '` | 1.1.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |
| Comprador con cédula; IVA 15 % + IVA 5 %; dos formas de pago; `otrosRubrosTerceros` | 2.1.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |
| Mínima a consumidor final | 1.0.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |
| Mínima a consumidor final | 2.0.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |
| Líneas exenta (7), no objeto (6) y gravada; propina | 1.1.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |
| Mínima firmada con **RSA-SHA1** | 1.1.0 | SHA-1 | `RECIBIDA` | **`AUTORIZADO`** |
| Contenido alterado después de firmar | 1.1.0 | SHA-256 | `RECIBIDA` | **`NO AUTORIZADO`**, error 39 «FIRMA INVALIDA» |
| … y luego el original, con la misma clave de acceso (ficha 5.10) | 1.1.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |
| Elemento no permitido por el XSD | 1.1.0 | — | **`DEVUELTA`**, error 35 «ARCHIVO NO CUMPLE ESTRUCTURA XML» con el detalle `cvc-complex-type.2.4.a` | — |
| … y luego el original, con la misma clave | 1.1.0 | SHA-256 | `RECIBIDA` | **`AUTORIZADO`** |

Conclusiones:

- El SRI autoriza **tanto RSA-SHA256 como RSA-SHA1**. El SDK usa SHA-256 por defecto (D2).
- Las cuatro versiones del esquema de factura se autorizan.
- El estado real de un comprobante no autorizado es el texto `NO AUTORIZADO` (la ficha muestra `RECHAZADO` en un ejemplo antiguo).
- Una firma inválida **no** se detecta en recepción (`RECIBIDA`) sino al autorizar: confirma que RECIBIDA no implica AUTORIZADO.
- Un comprobante devuelto o no autorizado puede reenviarse corregido con la misma clave de acceso y secuencial, como indica la ficha (5.10).
- El cálculo de IVA sobre la base acumulada (D5) y el redondeo del SDK no produjeron el error 52.

### Ronda 3 — 2026-11-15: nota de crédito

Para cada caso se emitió primero una factura de sustento (v1.1.0, comprador con RUC, IVA 15 % y 0 %), que quedó autorizada, y después la nota de crédito construida con `notacredito.DesdeFactura`.

| Escenario | Versión | Recepción | Autorización |
|---|---|---|---|
| Nota de crédito por el total de la factura | 1.1.0 | `RECIBIDA` | **`AUTORIZADO`** |
| Nota de crédito parcial (una línea, cantidad menor) | 1.1.0 | `RECIBIDA` | **`AUTORIZADO`** |

Se usó el XSD `NotaCredito_V1.1.0.xsd` (sin `valorDevolucionIva`). No se probó la versión 1.0.0 de la nota de crédito ni el campo de devolución de IVA.

Qué **no** demuestran estas rondas:

- Un solo certificado (Banco Central del Ecuador) y un solo emisor (persona natural). Certificados de otras entidades (Security Data, ANF, Uanataca…) pueden tener emisores con atributos distintos en `X509IssuerName`.
- Sin ICE, IRBPNR, exportación, reembolsos, retenciones en factura, negociable ni sustitutiva de guía de remisión.
- Sin la leyenda de negocio popular RIMPE (requiere un emisor de ese régimen).
- No se ha observado un comprobante en procesamiento prolongado ni el error 70.
- Solo el ambiente de pruebas, que puede validar menos que producción.

## Producción

No se envía ni se enviará nada a producción desde este proyecto.

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

### Ronda 4 — 2026-11-15: nota de débito

Factura de sustento autorizada (v1.1.0, USD 115,00) y, sobre ella, una nota de débito v1.0.0 con dos motivos (intereses y gastos de cobranza), IVA 15 % y una forma de pago.

| Escenario | Versión | Recepción | Autorización |
|---|---|---|---|
| Nota de débito con dos motivos e IVA 15 % | 1.0.0 | `RECIBIDA` | **`AUTORIZADO`** |

No se probó `valorDevolucionIva` (requiere el servicio DIG, Anexo 20).

### Ronda 5 — 2026-11-15: comprobante de retención

Comprobante de retención v2.0.0 (ATS) con un documento de sustento (factura de USD 200,00 más IVA 15 %), una retención de renta y una de IVA del 30 %, pago local.

| Intento | Recepción | Autorización |
|---|---|---|
| Renta con código 312 al 1,75 % | `RECIBIDA` | **`NO AUTORIZADO`**, error 52 «ERROR EN DIFERENCIAS»: «La tarifa (1.75) no es igual al parametrizado (2.0) para el código de porcentaje 312» |
| El mismo comprobante con el porcentaje corregido al 2 %, mismo secuencial y **nueva** clave de acceso | `RECIBIDA` | **`AUTORIZADO`** |

Conclusiones:

- El SRI contrasta el porcentaje de cada retención con su propia parametrización del código, que cambia con la normativa. A raíz de este rechazo el SDK incluye, además de la tabla de retención de IVA de la ficha (Tabla 20), el catálogo de retenciones de renta del ATS, que habría advertido de la discrepancia antes de enviar (ver `actualizar-catalogos.md`).
- Un secuencial que quedó `NO AUTORIZADO` puede volver a usarse con otra clave de acceso.
- El XSD de retención declara la firma con el prefijo `ns1:`; la firma del SDK (prefijo `ds:`, mismo espacio de nombres) es aceptada.

No se probaron: retenciones de ISD, pagos al exterior, dividendos, compra de banano, reembolsos ni la versión 1.0.0.

### Ronda 6 — 2026-11-15: guía de remisión

| Comprobante | Recepción | Autorización |
|---|---|---|
| Factura de sustento (cantidad con 6 decimales) | `RECIBIDA` | `AUTORIZADO` |
| Guía v1.1.0 con esa factura como sustento (número, clave de acceso y fecha), dos detalles, cantidad con 6 decimales, detalle adicional con `&` y `Ñ`, ruta | `RECIBIDA` | **`AUTORIZADO`** |
| Guía v1.0.0 sin comprobante de sustento, traslado entre establecimientos (`codEstabDestino`) | `RECIBIDA` | **`AUTORIZADO`** |

En ambas guías el transporte empieza el día de la emisión y termina al día siguiente, y la fecha de la clave de acceso es la de inicio del transporte. Los comprobantes devueltos son idénticos byte a byte a los enviados.

No se probaron: varios destinatarios, documento aduanero, emisión anterior al día del traslado (`GenerarClaveAccesoEn`) ni el rechazo 82.

### Ronda 7 — 2026-11-15: liquidación de compra

| Comprobante | Recepción | Autorización |
|---|---|---|
| Liquidación v1.1.0: proveedor identificado con cédula, un bien con IVA 0 % (cantidad con 6 decimales, unidad de medida) y un servicio con IVA 15 % | `RECIBIDA` | **`AUTORIZADO`** |
| Liquidación v1.0.0: un servicio con IVA 15 % y descuento | `RECIBIDA` | **`AUTORIZADO`** |

Los comprobantes devueltos son idénticos byte a byte a los enviados.

No se probaron: reembolsos (`codDocReembolso` 41), proveedores del exterior, liquidación negociable ni máquina fiscal.

Con esta ronda, los seis tipos de comprobante del esquema offline han sido autorizados al menos una vez en el ambiente de pruebas.

Qué **no** demuestran estas rondas:

- Un solo certificado (Banco Central del Ecuador) y un solo emisor (persona natural). Certificados de otras entidades (Security Data, ANF, Uanataca…) pueden tener emisores con atributos distintos en `X509IssuerName`.
- Sin ICE, IRBPNR, exportación, reembolsos, retenciones en factura, negociable ni sustitutiva de guía de remisión.
- Sin la leyenda de negocio popular RIMPE (requiere un emisor de ese régimen).
- No se ha observado un comprobante en procesamiento prolongado ni el error 70.
- Solo el ambiente de pruebas, que puede validar menos que producción.

## RIDE

Los RIDE no se envían al SRI; lo que sigue son comprobaciones locales sobre los comprobantes autorizados en las rondas anteriores (2026-11-15).

| Qué | Con qué | Resultado |
|---|---|---|
| RIDE de factura, nota de crédito, nota de débito, retención, guía de remisión y liquidación de compra | `ride.Generar` sobre los XML autorizados | Seis PDF de una página, de 3 a 4 KB |
| El PDF es válido y se ve como se espera | Dos visores independientes: poppler (`pdfinfo`, `pdftoppm`) y MuPDF (`mutool`), más revisión visual | Sin errores; contenido, tablas y totales correctos |
| El código de barras es legible | Lector zbar (`zbarimg`) sobre el PDF rasterizado a 300 ppp | Code 128 leído en los seis; coincide con la clave de acceso impresa |
| Correo con RIDE adjunto | Mensaje compuesto sin enviar | Dos adjuntos: XML y PDF |

No se ha impreso en papel ni se ha leído el código con un lector físico. No se ha enviado todavía un correo real con el RIDE adjunto.

## Correo

| Qué | Con qué | Resultado |
|---|---|---|
| Envío con texto, HTML y XML adjunto | Mailpit (contenedor local, sin cifrado) | Entregado; cabeceras, cuerpo y adjunto correctos |
| Entrega de una nota de crédito autorizada | Servidor SMTP real del propietario, puerto 587 con STARTTLS y autenticación (2026-11-15) | Mensaje aceptado por el servidor y **recibido en la bandeja de entrada** (Gmail), con el cuerpo y el XML adjunto correctos |

Que un servidor acepte el mensaje no garantiza la llegada a la bandeja de entrada: eso depende de la reputación del remitente (SPF, DKIM, DMARC), ajena al SDK.

## Producción

No se envía ni se enviará nada a producción desde este proyecto.

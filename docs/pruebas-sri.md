# Pruebas de interoperabilidad

Este documento resume qué se ha comprobado y en qué condiciones. Lo que no se ha probado se declara como tal. Resultados al **15 de noviembre de 2026**, con la ficha técnica v2.34.

## Condiciones

- **Ambiente:** certificación (pruebas) del SRI, `celcer.sri.gob.ec`. Nada se ha enviado a producción.
- **Emisor:** una persona natural habilitada en el ambiente de pruebas.
- **Certificado:** Banco Central del Ecuador, RSA 2048, con cadena de cuatro certificados en el `.p12`.
- Los datos del emisor y las claves de acceso no se publican.

Todos los comprobantes pasaron antes la validación local (reglas, XSD y verificación de la firma). En todos los autorizados, el comprobante que devuelve el SRI es **idéntico byte a byte** al enviado.

## Comprobantes autorizados por el SRI

| Comprobante | Versiones | Casos cubiertos |
|---|---|---|
| Factura | 1.0.0, 1.1.0, 2.0.0, 2.1.0 | Consumidor final, comprador con RUC y con cédula; IVA 15 %, 5 %, 0 %, exento y no objeto; descuento; precios y cantidades con 6 decimales; propina; crédito con plazo; varias formas de pago; rubros de terceros; detalles e información adicionales con `ñ & " < > '` |
| Factura de exportación (Anexo 4) | 1.1.0 | Comprador con identificación del exterior, incoterms, flete y seguro |
| Factura de reembolso de gastos (Anexo 5) | 1.1.0 | Un comprobante reembolsado con dos tarifas de IVA |
| Nota de crédito | 1.1.0 | Total y parcial, creadas con `notacredito.DesdeFactura` |
| Nota de débito | 1.0.0 | Dos motivos, IVA 15 %, una forma de pago |
| Comprobante de retención | 1.0.0, 2.0.0 | Retención de renta y de IVA sobre una factura; en la 2.0.0, un documento de sustento con pago local |
| Guía de remisión | 1.0.0, 1.1.0 | Con factura de sustento y sin ella (traslado entre establecimientos); dos detalles |
| Liquidación de compra | 1.0.0, 1.1.0 | Proveedor con cédula; IVA 0 % y 15 %; descuento; unidad de medida |

Firma: el SRI autoriza tanto **RSA-SHA256** (valor por defecto del SDK) como **RSA-SHA1**.

## Servicios y manejo de respuestas

| Qué | Resultado |
|---|---|
| Recepción y autorización individuales | Probadas con todos los comprobantes anteriores |
| Consulta de autorización por clave de acceso | `AUTORIZADO`, con fecha |
| Consulta de validez (`ConsultaComprobante`) | `estadoAutorizacion=AUTORIZADO` |
| Consulta de factura comercial negociable (`ConsultaFactura`) | El servicio responde y el SDK interpreta la respuesta; para una factura no negociable, `RECHAZADA` con el código 99 |
| Reenvío de un comprobante ya autorizado | El SRI responde `DEVUELTA` con el código 43; el SDK lo reconoce y obtiene la autorización |
| Comprobante alterado después de firmar | `RECIBIDA` y luego `NO AUTORIZADO`, código 39; el SDK lo reporta como no autorizado |
| Comprobante que incumple el XSD (enviado sin la validación local) | `DEVUELTA`, código 35 |
| Reenvío del comprobante correcto tras una devolución o un no autorizado | `AUTORIZADO` |
| Envío por lotes (`ws.Lote`) | Lote `RECIBIDA`; cada comprobante se resuelve por separado: en un lote con una factura válida y otra alterada, la primera quedó `AUTORIZADO` y la segunda `NO AUTORIZADO` |

## Comportamientos del SRI confirmados

Conviene conocerlos porque condicionan cómo se integra el SDK:

- **`RECIBIDA` no es autorización.** Una firma inválida no se detecta en la recepción sino al autorizar.
- El número de autorización es la clave de acceso.
- El estado de un comprobante rechazado en la autorización es el texto `NO AUTORIZADO`.
- Un comprobante devuelto o no autorizado puede reenviarse corregido con la misma clave de acceso. Su secuencial también puede reutilizarse con una clave nueva.
- El SRI contrasta el **porcentaje de cada retención** con el que tiene parametrizado para su código, y lo rechaza (código 52) si no coincide. El SDK incluye el catálogo de retenciones de renta y avisa antes de enviar; ese catálogo caduca (ver `actualizar-catalogos.md`).
- Los **códigos de país** son los de la Tabla 25 de la ficha (Catálogo ATS), no los de ISO 3166; el SRI rechaza los que no figuran en ella (código 52). El SDK incluye la tabla y avisa.
- El importe total de una factura de exportación incluye flete y seguro, como lo calcula el SDK.
- **Lotes:** el documento `<lote>` no lleva firma propia; su clave de acceso se compone con tipo de comprobante 01 y una serie que el emisor reserva para lotes.
- La firma del SDK usa el prefijo `ds:`; se acepta también en los comprobantes cuyo XSD declara la firma con `ns1:`.

## Verificación independiente

| Qué | Con qué | Resultado |
|---|---|---|
| Firma XAdES-BES, RSA-SHA256 y RSA-SHA1 | `xmlsec1` 1.3.6 | `Verification status: OK` |
| Validador XSD | `xmllint --schema` sobre los 13 XML de ejemplo del SRI | 13 de 13 veredictos idénticos a los del SDK |
| Claves de acceso | Las impresas en la ficha v2.34 | Todas verifican; el SDK reproduce la del Anexo 14 |
| RIDE de los seis comprobantes | Dos visores independientes (poppler y MuPDF) y revisión visual | PDF válidos; contenido, tablas y totales correctos |
| Código de barras del RIDE | Lector `zbar` sobre el PDF a 300 ppp | Code 128 legible en los seis; coincide con la clave de acceso |

`xmlsec1` valida XMLDSig (resúmenes y firma); no evalúa las propiedades XAdES.

## Correo

| Qué | Con qué | Resultado |
|---|---|---|
| Envío con texto, HTML y adjuntos | Mailpit (contenedor local) | Entregado; cabeceras, cuerpo y adjuntos correctos |
| Entrega de comprobantes autorizados con XML y RIDE adjuntos | Servidor SMTP real, puerto 587 con STARTTLS y autenticación | Recibidos en la bandeja de entrada (Gmail); el RIDE se abre correctamente, con logotipo y sin él |

Que un servidor acepte el mensaje no garantiza la llegada a la bandeja de entrada: depende de la reputación del remitente (SPF, DKIM, DMARC), ajena al SDK.

## Lo que no se ha probado

- **Producción.** El ambiente de pruebas puede validar menos que producción.
- **Otros certificados y emisores.** Solo un certificado (Banco Central del Ecuador) y un emisor (persona natural). Los certificados de otras entidades pueden tener emisores con atributos distintos en `X509IssuerName`.
- **Factura:** ICE, IRBPNR, retenciones informativas, una factura confirmada como comercial negociable, sustitutiva de guía de remisión, combustibles y transporte comercial (sus reglas solo tienen pruebas locales). Tampoco la leyenda de negocio popular RIMPE, que requiere un emisor de ese régimen.
- **Nota de crédito:** versión 1.0.0 y `valorDevolucionIva`. **Nota de débito:** `valorDevolucionIva`.
- **Retención:** ISD, pagos al exterior, dividendos, compra de banano y reembolsos.
- **Guía de remisión:** varios destinatarios, documento aduanero y emisión anterior al día del traslado.
- **Liquidación de compra:** reembolsos, proveedores del exterior y liquidación negociable.
- **Lotes:** otros tipos de comprobante o mezclados, el máximo de 50, y un comprobante que incumpla el XSD dentro de un lote (el simulador responde con el código 35 por comprobante; es una suposición).
- Un comprobante en procesamiento prolongado y el código 70.
- **RIDE:** impresión en papel y lectura del código con un lector físico.

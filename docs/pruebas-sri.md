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

Qué **no** demuestra esta ronda:

- **RSA-SHA1 no se envió al SRI.** Tras autorizarse SHA-256 no hizo falta; su aceptación real sigue sin probar con este SDK.
- Un solo certificado (BCE) y un solo tipo de emisor. Certificados de otras entidades (Security Data, ANF, Uanataca…) pueden tener emisores con atributos distintos en `X509IssuerName`.
- Solo factura 1.1.0 mínima: sin ICE, descuentos, crédito, exportación ni las versiones 1.0.0 / 2.x.
- No se ha visto un `NO AUTORIZADO` real ni un comprobante en procesamiento prolongado.

## Producción

No se envía ni se enviará nada a producción desde este proyecto.

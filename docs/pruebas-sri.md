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

**Pendiente.** Hasta ahora solo se han descargado los WSDL públicos; no se ha enviado ningún comprobante.

Plan de la primera ronda (requiere autorización del titular del certificado):

| # | Experimento | Pregunta que responde |
|---|---|---|
| 1 | Factura mínima v1.1.0, firma RSA-SHA1 | ¿El SRI recibe y **autoriza** la firma del SDK con el algoritmo normativo? |
| 2 | Igual, firma RSA-SHA256 | ¿SHA-256 es aceptado? (decide la recomendación D2) |
| 3 | Reenvío del comprobante 1 ya autorizado | ¿Responde 43 como se supone en D8? |
| 4 | Consulta de validez de la clave 1 | Forma real de la respuesta de `ConsultaComprobante` |

Para cada uno se registrará por separado: validación local, firma verificada, RECIBIDA y AUTORIZADO, con las respuestas sanitizadas.

## Producción

No se envía ni se enviará nada a producción desde este proyecto.

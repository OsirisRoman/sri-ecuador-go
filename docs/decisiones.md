# Decisiones técnicas

Cada entrada indica qué se decidió, por qué y qué alternativa se descartó. Las marcadas **(revisable)** dependen de resultados de pruebas reales.

## D1. Árbol XML y canonicalización propios
`encoding/xml` descarta los prefijos y no canonicaliza. En lugar de sumar `etree` + `goxmldsig` (como las referencias en Go), `internal/xmldom` implementa en ~450 líneas un árbol que conserva prefijos y la canonicalización C14N 1.0 inclusiva, que es la única que usa el perfil del SRI.
*Verificación:* casos de la recomendación C14N, y firmas validadas por una implementación independiente (`xmlsec1`/OpenSSL).

## D2. Firma: estructura del Anexo 14, RSA-SHA1 por defecto (revisable)
La ficha (6.8) dice literalmente «Algoritmo de firmado: RSA-SHA1» y el Anexo 14 muestra SHA-1 en firma y digest. SHA-1 está en desuso criptográfico, y al menos una referencia (Open-SRI, vía `xades4j`) firma con SHA-256. Se implementan ambos; **el valor por defecto es el normativo (SHA-1)** hasta contar con evidencia real del ambiente de certificación. Ver `docs/pruebas-sri.md`.
La firma lleva las tres referencias del Anexo 14 (SignedProperties, KeyInfo y comprobante) y nunca se entrega sin pasar la verificación del propio SDK.

## D3. `X509IssuerName` con formato RFC 2253 al estilo Java
El SRI valida con bibliotecas Java. `pkix.Name.String()` de Go usa palabras clave (`SERIALNUMBER`, `POSTALCODE`) que Java no emite. `xades.NombreRFC2253` trabaja sobre el DER del emisor y reproduce la salida de `X500Principal.getName(RFC2253)`: solo las palabras clave de RFC 2253 y el resto como OID con valor hexadecimal.

## D4. Decimales propios
Un entero de precisión arbitraria con escala. Sin dependencia externa porque solo hacen falta suma, resta, producto, porcentaje y redondeo. Redondeo «mitad hacia arriba».

## D5. Total de IVA sobre la base acumulada
`Calcular` obtiene el IVA de cada grupo como `suma de bases × tarifa`, no como suma de los IVA de cada línea. `Validar` acepta ambos criterios, porque los dos son de uso corriente y difieren en centavos.

## D6. No se exige el dígito verificador del RUC
El SRI asigna RUC que no cumplen el módulo 11 clásico; exigirlo rechazaría contribuyentes reales. Se valida forma y provincia. La cédula con dígito incorrecto es *advertencia*, igual que en el SRI (código 62).

## D7. Ajustes a los XSD publicados, explícitos y en memoria
El XSD publicado solo admite `CONTRIBUYENTE RÉGIMEN RIMPE`, pero el Anexo 22 exige además `CONTRIBUYENTE NEGOCIO POPULAR - RÉGIMEN RIMPE`. Los archivos se incrustan intactos; la corrección se aplica al cargar y está declarada en `esquema.Ajustes`. `esquema.ValidarEstricto` usa los archivos literales. **(revisable)**: falta confirmar con el SRI real.

## D8. Reintentar el envío es seguro
Reenviar un comprobante ya recibido no lo duplica: el SRI responde 43 o 70. El cliente lo interpreta como «ya recibido» y pasa a consultar la autorización, lo que hace idempotente a `Reanudar`.

## D9. El ambiente es obligatorio y se comprueba
`sri.Config.Ambiente` no tiene valor por defecto y el cliente rechaza comprobantes de otro ambiente. Un ambiente desconocido nunca resuelve a las direcciones de producción.

## D10. Nombres en español para el dominio
Tipos y campos siguen las etiquetas XML del SRI (`InfoTributaria`, `RazonSocial`), de modo que el código se lee junto a la ficha técnica.

## D11. `comprobante.Lista[T]`
`encoding/xml` escribe `<pagos></pagos>` para un slice vacío con `pagos>pago,omitempty`, y el XSD lo rechaza. `Lista[T]` es un slice normal cuya serialización omite el contenedor vacío.

## Discrepancias abiertas
- **Notas de crédito/débito:** el SRI publica dos familias de XSD incompatibles (una admite `agenteRetencion`/`contribuyenteRimpe`, la otra `valorDevolucionIva`). Pendiente de resolver al implementar esos comprobantes.
- **Estados de autorización:** la ficha muestra `RECHAZADO` en un ejemplo y habla de «NO AUTORIZADO» en el texto. El cliente trata como no autorizado todo estado resuelto que no sea `AUTORIZADO`.

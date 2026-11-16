# Vectores de prueba

Datos reproducibles para comprobar una implementación del protocolo del SRI, en cualquier lenguaje, sin necesidad de un certificado real ni de conexión.

Todos los datos son **ficticios**: el emisor `1790000000001` no existe, el certificado es desechable y lo emite una autoridad inventada (`AC DE PRUEBA GOSRI`), y el receptor es el nombre de pruebas que sugiere la ficha técnica. Ninguno de estos documentos tiene validez tributaria ni sería autorizado por el SRI.

Los genera y los comprueba [`internal/vectores`](../internal/vectores/vectores_test.go); las pruebas del proyecto fallan si dejan de coincidir con lo que produce el SDK.

| Archivo | Para comprobar |
|---|---|
| `clave-acceso.json` | La composición de la clave de acceso y el dígito verificador módulo 11. Cada caso trae los componentes, los 48 dígitos base, el dígito y la clave. Incluye la clave del Anexo 14 de la ficha y los dos casos límite (dígitos 0 y 1) |
| `calculo-factura.json` | El cálculo de totales: por cada caso, las entradas de cada línea y todos los importes esperados |
| `precio-final.json` | El cálculo de un detalle definido por su precio final con IVA incluido: por cada línea, las entradas (cantidad, precio final, descuento, código de IVA) y lo que se deduce (importe, base, IVA, precio unitario), junto con el IVA que daría la misma base con la fórmula por defecto |
| `firma/factura.xml` | Un comprobante sin firmar, que cumple el XSD, con caracteres especiales (`ñ & " < > '`) |
| `firma/factura.firmada.sha256.xml` | El mismo comprobante firmado con XAdES-BES y RSA-SHA256 |
| `firma/factura.firmada.sha1.xml` | El mismo comprobante firmado con RSA-SHA1 |
| `firma/c14n-comprobante.xml` | Forma canónica del comprobante sin la firma: lo que se resume en la referencia `#comprobante` |
| `firma/c14n-signedproperties.xml` | Forma canónica de `etsi:SignedProperties` dentro del documento |
| `firma/c14n-keyinfo.xml` | Forma canónica de `ds:KeyInfo` dentro del documento |
| `firma/c14n-signedinfo.xml` | Forma canónica de `ds:SignedInfo`: lo que se firma |
| `firma/firma.sha256.json` | Datos intermedios de la firma SHA-256: los tres resúmenes en Base64, el emisor del certificado en DER (hexadecimal), el `X509IssuerName` esperado y el número de serie |

## Cómo usarlos

**Verificador de firmas.** Su implementación debe dar por válidas las dos facturas firmadas. Para la vigencia del certificado use como instante de referencia el `SigningTime` (`2026-11-14T10:00:00-05:00`); el certificado es válido de 2020 a 2040.

**Canonicalizador.** A partir de `factura.firmada.sha256.xml`, canonicalice cada pieza y compárela byte a byte con el archivo `c14n-*.xml` correspondiente. Fíjese en que `c14n-signedproperties.xml` y `c14n-keyinfo.xml` empiezan declarando los espacios de nombres `ds` y `etsi`, heredados de `ds:Signature`.

**Firmante.** No podrá reproducir `SignatureValue` (no se publica la clave privada, y cada firma lleva identificadores aleatorios), pero sí comprobar los pasos previos: el SHA-256 de cada archivo `c14n-*.xml`, en Base64, debe ser el `DigestValue` de su referencia, que figura en `firma.sha256.json` y en `c14n-signedinfo.xml`. Después, firme su propio documento y verifíquelo con una herramienta independiente como `xmlsec1`, o envíelo al simulador `sritest`.

**Nombre del emisor.** Convierta `emisorDelCertificadoDERHex` a texto RFC 2253 y compare con `x509IssuerName`. El emisor de prueba incluye a propósito un atributo `organizationIdentifier` (2.5.4.97), que debe escribirse como OID y valor DER en hexadecimal.

**Serialización.** `factura.firmada.*.xml` empieza exactamente por el contenido de `factura.xml` (salvo las comillas del texto, que tras firmar quedan literales, una forma equivalente): la firma se añade al final sin tocar el resto.

La explicación de cada algoritmo está en [`docs/referencia/`](../docs/referencia/README.md).

## Regenerarlos

```sh
GENERAR_VECTORES=1 go test ./internal/vectores
```

Las firmas cambian en cada regeneración, porque el certificado desechable y los identificadores son nuevos.

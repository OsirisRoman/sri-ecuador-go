# Firma XAdES-BES

Es la parte más delicada de una implementación. Esta página describe la firma que este SDK genera y que el SRI autorizó en pruebas, con el detalle necesario para reproducirla byte a byte. Los vectores de [`vectores/firma/`](../../vectores/README.md) permiten comprobar cada resumen por separado.

**Regla oficial:** ficha técnica v2.34, sección 6 y Anexo 14. Estándares: ETSI TS 101 903 v1.3.2 (XAdES), XMLDSig del W3C, Canonical XML 1.0 y RFC 2253.

## Requisitos

- Firma **XAdES-BES**, **enveloped**: el elemento `ds:Signature` es el último hijo del elemento raíz del comprobante.
- Certificado X.509 de una entidad de certificación autorizada en Ecuador, con clave RSA.
- Una sola firma por comprobante.

## Estructura

```xml
<factura id="comprobante" version="1.1.0">
  …
  <ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"
                xmlns:etsi="http://uri.etsi.org/01903/v1.3.2#" Id="Signature{n}">
    <ds:SignedInfo Id="Signature-SignedInfo{n}">
      <ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"/>
      <ds:SignatureMethod Algorithm="…rsa-sha256"/>
      <ds:Reference Id="SignedPropertiesID{n}" Type="http://uri.etsi.org/01903#SignedProperties"
                    URI="#Signature{n}-SignedProperties{n}">
        <ds:DigestMethod Algorithm="…sha256"/><ds:DigestValue>…</ds:DigestValue>
      </ds:Reference>
      <ds:Reference URI="#Certificate{n}">
        <ds:DigestMethod Algorithm="…sha256"/><ds:DigestValue>…</ds:DigestValue>
      </ds:Reference>
      <ds:Reference Id="Reference-ID-{n}" URI="#comprobante">
        <ds:Transforms><ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"/></ds:Transforms>
        <ds:DigestMethod Algorithm="…sha256"/><ds:DigestValue>…</ds:DigestValue>
      </ds:Reference>
    </ds:SignedInfo>
    <ds:SignatureValue Id="SignatureValue{n}">…</ds:SignatureValue>
    <ds:KeyInfo Id="Certificate{n}">
      <ds:X509Data><ds:X509Certificate>…</ds:X509Certificate></ds:X509Data>
      <ds:KeyValue><ds:RSAKeyValue><ds:Modulus>…</ds:Modulus><ds:Exponent>…</ds:Exponent></ds:RSAKeyValue></ds:KeyValue>
    </ds:KeyInfo>
    <ds:Object Id="Signature{n}-Object{n}">
      <etsi:QualifyingProperties Target="#Signature{n}">
        <etsi:SignedProperties Id="Signature{n}-SignedProperties{n}">
          <etsi:SignedSignatureProperties>
            <etsi:SigningTime>2026-11-14T10:00:00-05:00</etsi:SigningTime>
            <etsi:SigningCertificate><etsi:Cert>
              <etsi:CertDigest><ds:DigestMethod Algorithm="…sha256"/><ds:DigestValue>…</ds:DigestValue></etsi:CertDigest>
              <etsi:IssuerSerial>
                <ds:X509IssuerName>…</ds:X509IssuerName><ds:X509SerialNumber>…</ds:X509SerialNumber>
              </etsi:IssuerSerial>
            </etsi:Cert></etsi:SigningCertificate>
          </etsi:SignedSignatureProperties>
          <etsi:SignedDataObjectProperties>
            <etsi:DataObjectFormat ObjectReference="#Reference-ID-{n}">
              <etsi:Description>contenido comprobante</etsi:Description>
              <etsi:MimeType>text/xml</etsi:MimeType>
            </etsi:DataObjectFormat>
          </etsi:SignedDataObjectProperties>
        </etsi:SignedProperties>
      </etsi:QualifyingProperties>
    </ds:Object>
  </ds:Signature>
</factura>
```

`{n}` son números distintos, elegidos al azar por el firmante; solo importa que los identificadores sean únicos y que las referencias apunten a ellos.

## Las tres referencias

`SignedInfo` contiene tres referencias, en este orden. Las tres son obligatorias: una firma que solo cubre el comprobante no es la que describe el Anexo 14.

| # | Qué cubre | URI | Transformaciones |
|---|---|---|---|
| 1 | `etsi:SignedProperties` | `#` + su `Id`, con `Type="http://uri.etsi.org/01903#SignedProperties"` | ninguna explícita |
| 2 | `ds:KeyInfo` | `#` + su `Id` | ninguna explícita |
| 3 | El comprobante completo | `#comprobante` | `enveloped-signature` |

## Cómo se calcula cada resumen

Todos los resúmenes se calculan sobre la **forma canónica C14N 1.0 inclusiva, sin comentarios** del elemento referenciado, **tal como queda dentro del documento ya armado**. Esto último es la causa más frecuente de firmas inválidas: en la canonicalización inclusiva, un elemento escribe las declaraciones de espacios de nombres que están en vigor en su contexto, aunque se hayan declarado en un antecesor. Por eso:

- La forma canónica de `SignedProperties` y de `KeyInfo` empieza declarando `xmlns:ds` y `xmlns:etsi` (heredados de `ds:Signature`), en orden alfabético de prefijo.
- Si la firma se calcula con esos elementos sueltos, fuera del documento, los resúmenes no coinciden con los que calculará el SRI.

Pasos:

1. Añada `ds:Signature`, con `KeyInfo` y `Object` completos, como último hijo de la raíz.
2. **Resumen del comprobante (referencia 3):** canonicalice el elemento raíz **omitiendo por completo** el elemento `ds:Signature` (eso es la transformación *enveloped*) y calcule el resumen.
3. **Resumen de `SignedProperties` (referencia 1)** y **de `KeyInfo` (referencia 2):** canonicalice cada elemento en su sitio y calcule el resumen.
4. Construya `SignedInfo` con los tres resúmenes en Base64.
5. Canonicalice `SignedInfo` en su sitio, calcule su resumen y fírmelo con la clave privada: **RSASSA-PKCS1-v1_5**. El resultado, en Base64, es `SignatureValue`.
6. Serialice el documento **sin alterar nada** de lo ya resumido: ni espacios, ni orden de atributos, ni escapes.

**Decisión de este SDK:** serializar el documento firmado directamente en forma canónica, de modo que los bytes enviados son exactamente los resumidos, y verificar la firma recién generada antes de devolverla.

### Recordatorio de C14N 1.0

- Sin declaración XML; UTF-8.
- Los elementos vacíos se escriben con etiqueta de apertura y de cierre (`<a></a>`).
- Atributos ordenados: primero las declaraciones de espacios de nombres (por prefijo) y después los atributos (por nombre).
- En texto se escapan `&`, `<`, `>` y el retorno de carro; en atributos, `&`, `<`, `"`, tabulador, salto de línea y retorno de carro. Las comillas simples y dobles del texto quedan literales.
- No se añade ni se quita ningún espacio en blanco.

## Algoritmos

| | RSA-SHA256 (por defecto en este SDK) | RSA-SHA1 |
|---|---|---|
| `SignatureMethod` | `http://www.w3.org/2001/04/xmldsig-more#rsa-sha256` | `http://www.w3.org/2000/09/xmldsig#rsa-sha1` |
| `DigestMethod` | `http://www.w3.org/2001/04/xmlenc#sha256` | `http://www.w3.org/2000/09/xmldsig#sha1` |

**Regla oficial:** la ficha (6.8) nombra RSA-SHA1 y su Anexo 14 lo usa. **Comprobado:** el SRI autoriza comprobantes firmados con cualquiera de los dos. **Decisión de este SDK:** SHA-256 por defecto, porque SHA-1 está en desuso criptográfico.

El resumen de `CertDigest` se calcula sobre el certificado en DER (los mismos bytes que, en Base64, van en `X509Certificate`).

## `X509IssuerName`

Es el nombre del emisor del certificado en texto **RFC 2253, en el formato que produce Java** (`X500Principal.getName()`), que es la plataforma con la que el SRI valida:

- Los atributos se escriben **en orden inverso** al del certificado, separados por comas sin espacio.
- Solo se usan estas palabras clave: `CN`, `L`, `ST`, `O`, `OU`, `C`, `STREET`, `DC`, `UID`.
- **Cualquier otro atributo se escribe como OID numérico, `=`, `#` y su valor DER en hexadecimal.** Es frecuente en certificados ecuatorianos: `serialNumber` (2.5.4.5) y `organizationIdentifier` (2.5.4.97). Ejemplo: `2.5.4.97=#131356415445432d31373930303030303030303031,CN=…`.
- Se escapan con `\` los caracteres `, + " \ < > ;`, un `#` o un espacio inicial y un espacio final.

Convertir el nombre con las utilidades habituales de otros lenguajes (que escriben `serialNumber=…` o reordenan atributos) da un texto distinto. `vectores/firma/firma.sha256.json` incluye el emisor en DER y el texto esperado.

`X509SerialNumber` es el número de serie en decimal.

## Otros detalles

- `SigningTime` se escribe con el desplazamiento de Ecuador continental (`-05:00`).
- `Modulus` y `Exponent` van en Base64 de su representación binaria sin ceros iniciales.
- El elemento raíz debe conservar `id="comprobante"`.

## Verificación

Una verificación completa comprueba: que exista una única firma y sea hija de la raíz; que las tres referencias existan y sus resúmenes coincidan; que la referencia al comprobante cubra la raíz; que `SignatureValue` verifique con la clave del certificado de `KeyInfo`; que `CertDigest` e `IssuerSerial` correspondan a ese certificado; y que el certificado estuviera vigente en `SigningTime`. Este SDK no consulta revocación (OCSP ni CRL); el SRI sí valida el certificado al autorizar.

**Comprobado:** una firma inválida no se detecta en la recepción (`RECIBIDA`), sino en la autorización (`NO AUTORIZADO`, código 39).

## Lo que no está comprobado

- Certificados de entidades distintas del Banco Central del Ecuador (Security Data, ANF, Uanataca y otras). Sus emisores pueden tener atributos distintos en `X509IssuerName`; la regla anterior debería cubrirlos, pero no se ha probado con el SRI.
- Certificados con claves que no sean RSA.
- Firma con dispositivos criptográficos (tokens USB, HSM).

Implementación: [`xades`](../../xades) y [`internal/xmldom`](../../internal/xmldom) (árbol XML y canonicalizador propios).

# Notas de versión

Los cambios notables de cada versión. El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y las versiones, [SemVer](https://semver.org/lang/es/), según la [política de versiones](docs/versionado.md). Mientras la versión sea 0.x, la API puede cambiar entre versiones menores.

## Sin publicar

- Documentación: política de versiones (`docs/versionado.md`).

## v0.1.0 — primera versión pública (pre-release) — 2026-11-15

Primera versión utilizable del SDK. Es una versión previa a la 1.0: funciona y está probada contra el ambiente de pruebas del SRI, pero la API puede cambiar y hay casos sin verificar, que se declaran abajo.

### Requisitos

- Go 1.26 o superior. Sin cgo ni programas externos.
- Para emitir: un certificado de firma electrónica en archivo `.p12` de una entidad autorizada en Ecuador y un RUC habilitado en el ambiente correspondiente del SRI.
- Especificación de referencia: *Ficha técnica de comprobantes electrónicos esquema offline* v2.34 (julio 2026), vigente al 15 de noviembre de 2026.

### Funcionalidades

**Comprobantes** (modelo tipado, lectura y escritura de XML, cálculo de totales, clave de acceso y validación):

- Factura 1.0.0, 1.1.0, 2.0.0 y 2.1.0, con exportación, reembolsos y requisitos de la factura comercial negociable.
- Nota de crédito 1.0.0 y 1.1.0; `notacredito.DesdeFactura` revierte una factura.
- Nota de débito 1.0.0.
- Comprobante de retención 2.0.0 (ATS) y 1.0.0, con el catálogo de retenciones de renta.
- Guía de remisión 1.0.0 y 1.1.0.
- Liquidación de compra 1.0.0 y 1.1.0.

**Base**

- Decimales exactos (`decimal`) y clave de acceso con módulo 11 (`clave`).
- Validación contra los XSD oficiales del SRI, incrustados (`esquema`).
- Reglas de negocio con identificadores estables, que distinguen errores de advertencias.
- Catálogos de la ficha: tarifas de IVA, formas de pago, países, retenciones.

**Firma y servicios**

- Carga de certificados PKCS#12 (`certificado`).
- Firma y verificación XAdES-BES con RSA-SHA256 (por defecto) y RSA-SHA1 (`xades`).
- Clientes SOAP de recepción, autorización, lotes, consulta de validez y consulta de factura comercial negociable (`ws`).
- Flujo de emisión con reintentos seguros y reanudación (`sri`).
- Simulador del SRI y certificado desechable para pruebas (`sritest`).

**Entrega**

- RIDE en PDF de los seis comprobantes, con código de barras y logotipo opcional (`ride`).
- Correo de entrega con XML y RIDE, opcional (`correo`).
- Herramienta de línea de comandos (`cmd/sri`).

**Documentación**

- Guía de integración, ejemplo ejecutable e instrucciones para asistentes de IA.
- Referencia técnica independiente del lenguaje y vectores de prueba.
- Capacidades con su estado, pendientes documentados y guía de contribución.

### Compatibilidad demostrada

Autorizado por el **ambiente de pruebas del SRI** (un emisor, certificado del Banco Central del Ecuador):

- Factura en sus cuatro versiones, con RSA-SHA256 y RSA-SHA1; factura de exportación y de reembolso de gastos.
- Nota de crédito 1.1.0 (total y parcial), nota de débito 1.0.0.
- Comprobante de retención 2.0.0 y 1.0.0.
- Guía de remisión 1.0.0 y 1.1.0; liquidación de compra 1.0.0 y 1.1.0.
- Envío por lotes, consulta de autorización y de validez, y manejo de los códigos 35, 39 y 43.

El detalle está en [`docs/pruebas-sri.md`](docs/pruebas-sri.md).

### Limitaciones

- **No se ha probado en producción** ni con certificados de otras entidades de certificación.
- **Devolución automática de IVA (servicios DIG): no implementada.** El SDK rechaza un `valorDevolucionIva` mayor que cero, porque no descuenta ese valor del importe total (Anexo 20).
- Solo con pruebas locales: ICE e IRBPNR, factura comercial negociable confirmada, sustitutiva de guía de remisión, casos de retención distintos del pago local (ISD, exterior, dividendos), nota de crédito 1.0.0, entre otros.
- La verificación de firmas no consulta la revocación de certificados. No se admiten tokens ni HSM.
- El RIDE no incrusta fuentes ni es PDF/A; los caracteres fuera de Windows-1252 se sustituyen.
- Los catálogos incluidos caducan con los cambios normativos.
- El SDK no asigna secuenciales, no almacena comprobantes y no anula comprobantes.

Cada limitación está explicada, con lo que falta para resolverla, en [`docs/pendientes.md`](docs/pendientes.md).

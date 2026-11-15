# Capacidades y limitaciones

Leyenda: ✅ implementado y probado · 🚧 parcial · ⏳ pendiente.

| Capacidad | Estado | Notas |
|---|---|---|
| Clave de acceso (generar, analizar, validar) | ✅ | Comprobada con las claves impresas en la ficha v2.34 |
| Decimales exactos | ✅ | |
| Factura: modelo tipado v1.0.0 / 1.1.0 / 2.0.0 / 2.1.0 | ✅ | Incluye exportación, reembolsos, retenciones, rubros de terceros, sustitutiva de guía, negociable, máquina fiscal |
| Factura: serialización y lectura XML | ✅ | Ida y vuelta sin pérdida sobre los ejemplos oficiales del SRI |
| Factura: cálculo de totales | ✅ | IVA; ICE/IRBPNR ad valorem. Tarifas específicas de ICE: el usuario indica el valor |
| Factura: reglas de validación | 🚧 | Núcleo completo. Sin reglas específicas aún para exportación, reembolsos, combustibles (Anexos 12/16), transporte (25) |
| Validación XSD | ✅ | 16 esquemas oficiales incrustados (todos los comprobantes) |
| Certificados PKCS#12 | ✅ | Cifrados moderno y heredados; archivos con varias claves. Sin tokens USB/PKCS#11 |
| Firma XAdES-BES | ✅ | SHA-256 (por defecto) y SHA-1, ambas autorizadas por el SRI; verificada con xmlsec1 |
| Verificación de firma | ✅ | Integridad, cobertura, certificado; cadena de confianza opcional. Sin OCSP/CRL |
| SOAP recepción y autorización | ✅ | Probado con el SRI real (pruebas), con las respuestas de la ficha y con el simulador |
| SOAP consulta de validez | ✅ | Probado con el SRI real para un comprobante autorizado |
| SOAP lotes, factura comercial negociable | ⏳ | |
| Reintentos e idempotencia | ✅ | |
| Simulador del SRI (`sritest`) | ✅ | |
| **Interoperabilidad con el SRI real** | 🚧 | Facturas 1.0.0, 1.1.0, 2.0.0 y 2.1.0 **autorizadas** en el ambiente de pruebas (SHA-256 y SHA-1, certificado BCE). Alcance y límites en `pruebas-sri.md` |
| Nota de crédito v1.0.0 / 1.1.0 | ✅ | Modelo, totales, validación y `DesdeFactura`. Total y parcial **autorizadas** en el ambiente de pruebas (1.1.0). Sin `valorDevolucionIva` (Anexo 20) |
| Nota de débito v1.0.0 | ✅ | Modelo, totales y validación. **Autorizada** en el ambiente de pruebas. Admite `valorDevolucionIva`, sin probar |
| Comprobante de retención v2.0.0 (ATS) | ✅ | Modelo completo, cálculo y validación; tablas de retención de IVA y de renta (Catálogo ATS, 127 conceptos; **caduca**, ver `actualizar-catalogos.md`). **Autorizado** en el ambiente de pruebas. La v1.0.0 no se modela |
| Guía de remisión v1.0.0 / 1.1.0 | ✅ | Modelo, clave de acceso (fecha de emisión distinta del inicio del transporte), validación; autorizada en pruebas |
| Liquidación de compra | ⏳ | XSD ya incrustados; falta el modelo |
| RIDE (representación impresa) | ⏳ | |
| Envío por correo (`correo`) | 🚧 | Paquete opcional, solo biblioteca estándar. STARTTLS/TLS obligatorios fuera de localhost. Probado contra Mailpit y con un servidor SMTP real (STARTTLS); adjunta el XML autorizado (el RIDE cuando exista) |
| Devolución de IVA (servicios DIG, sección 13) | ⏳ | |

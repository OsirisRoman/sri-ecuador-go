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
| Firma XAdES-BES | ✅ | SHA-1 y SHA-256; verificada con xmlsec1 |
| Verificación de firma | ✅ | Integridad, cobertura, certificado; cadena de confianza opcional. Sin OCSP/CRL |
| SOAP recepción y autorización | ✅ | Probado con las respuestas literales de la ficha y con el simulador |
| SOAP consulta de validez | 🚧 | Implementado; sin prueba contra el SRI real |
| SOAP lotes, factura comercial negociable | ⏳ | |
| Reintentos e idempotencia | ✅ | |
| Simulador del SRI (`sritest`) | ✅ | |
| **Interoperabilidad con el SRI real** | ⏳ | Ver `pruebas-sri.md` |
| Nota de crédito, nota de débito, guía de remisión, retención, liquidación de compra | ⏳ | XSD ya incrustados; faltan los modelos |
| RIDE (representación impresa) | ⏳ | |
| Envío por correo | ⏳ | Será opcional, fuera del núcleo |
| Devolución de IVA (servicios DIG, sección 13) | ⏳ | |

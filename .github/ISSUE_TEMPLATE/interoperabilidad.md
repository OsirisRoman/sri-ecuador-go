---
name: Informe de interoperabilidad
about: Resultado de usar el SDK contra el SRI (funcionó o no)
labels: interoperabilidad
---

Estos informes amplían lo que se sabe que funciona. Ver «Aportar pruebas de interoperabilidad sin exponer datos» en CONTRIBUTING.md.

- **Ambiente:** pruebas / producción
- **Fecha:**
- **Versión del SDK:**
- **Comprobante y versión:** (por ejemplo, factura 1.1.0)
- **Caso:** (por ejemplo, con ICE, exportación, varios destinatarios)
- **Entidad que emitió el certificado:** (solo el nombre de la entidad)
- **Algoritmo de firma:** RSA-SHA256 / RSA-SHA1
- **Resultado:** AUTORIZADO / NO AUTORIZADO / DEVUELTA
- **Mensajes del SRI:** (código y texto)

**Detalles útiles**
Si falló la firma, el contenido de `X509IssuerName` que generó el SDK.

<!-- No incluya el certificado, contraseñas, claves de acceso reales, RUC, cédulas ni nombres reales. -->

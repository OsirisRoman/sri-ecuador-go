# Seguridad

## Reportar una vulnerabilidad

No abra un *issue* público. Use el aviso privado de GitHub: pestaña **Security** del repositorio → **Report a vulnerability**.

Incluya qué versión o *commit* afecta, cómo reproducirlo y qué impacto tiene. No adjunte certificados, contraseñas ni comprobantes reales: use datos ficticios.

Este proyecto lo mantienen voluntarios; se responderá en cuanto sea posible, sin plazos garantizados. Se agradece no divulgar el problema hasta que haya una corrección publicada.

## Versiones con correcciones

Mientras el proyecto esté en versiones 0.x, las correcciones de seguridad se publican solo en la versión más reciente.

## Qué protege el SDK y qué no

El SDK:

- No escribe la clave privada ni la contraseña del certificado en registros ni en mensajes de error.
- Rechaza documentos XML con DOCTYPE y limita su tamaño y profundidad.
- Exige indicar el ambiente y se niega a enviar un comprobante de un ambiente por el cliente de otro.
- Exige TLS en el envío de correo fuera de `localhost`.
- No tiene dependencias con cgo y se analiza con `govulncheck`.

Es responsabilidad de quien lo integra:

- Custodiar el archivo `.p12` y su contraseña (gestor de secretos, permisos restrictivos) y mantenerlos fuera del repositorio.
- Controlar quién puede emitir comprobantes con su aplicación.
- Proteger los comprobantes almacenados, que contienen datos personales y tributarios.
- Mantener el SDK y Go actualizados.

`xades.Verificar` no consulta la revocación de certificados (OCSP ni CRL). La firma RSA-SHA1 existe porque la ficha técnica del SRI la nombra; el valor por defecto es RSA-SHA256.

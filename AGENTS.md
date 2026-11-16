# Instrucciones para asistentes de IA

Este archivo orienta a un asistente de IA (y a cualquier persona con prisa) que trabaje con `sri-ecuador-go`, un SDK en Go para la facturación electrónica del SRI de Ecuador (esquema offline).

## Si vas a USAR el SDK en otra aplicación

Lee primero [`docs/guia-de-integracion.md`](docs/guia-de-integracion.md) y toma como modelo [`ejemplos/emision/main.go`](ejemplos/emision/main.go), que compila y se ejecuta sin configuración (`go run ./ejemplos/emision`). No inventes funciones: comprueba los nombres con `go doc github.com/OsirisRoman/sri-ecuador-go/<paquete>`.

Lo esencial, que no debes saltarte:

- Importes con `decimal.Decimal`, nunca `float64`.
- Orden: llenar la estructura → `Calcular()` → `GenerarClaveAcceso("")` → `cliente.Preparar` → guardar clave y XML firmado → `cliente.Reanudar` → actuar según `res.Estado`.
- Solo `sri.EstadoAutorizado` es un comprobante válido. `RECIBIDO` es pendiente (se resuelve con `cliente.ConsultarDocumento`); un error de red o `sri.ErrEstadoIndeterminado` es estado desconocido y se reintenta con el **mismo** XML firmado, sin generar otra clave ni otro secuencial.
- `sri.EstadoDiscrepancia` (`errors.Is(err, sri.ErrDiscrepancia)`) significa que el SRI tiene **otro** comprobante con esa clave: no entregues el XML enviado; el del SRI está en `res.Autorizacion`.
- Un comprobante devuelto o no autorizado se corrige y se reenvía con la **misma** clave y el mismo secuencial (ficha 5.10): no vuelvas a llamar a `GenerarClaveAcceso`.
- Después de cambiar una cantidad, un precio o una tarifa, vuelva a llamar a `Calcular`: recalcula todo lo que dedujo. La base o el valor de un ICE específico y de un IRBPNR los indica y los actualiza la aplicación.
- Con `Reanudar`, `Emitir` y `ConsultarDocumento`, `sri.EstadoAutorizado` garantiza que el documento autorizado es el enviado (`res.Verificado`). `cliente.Autorizar(clave)` solo informa el estado de la clave. `sri.ErrSinComprobante` es un estado desconocido.
- Precios: `factura.LineaIVA` recibe el precio **sin** impuestos; si el precio pactado ya incluye el IVA, use `factura.LineaPrecioFinal` (el comprobante suma exactamente ese precio). No convierta a mano un precio final en una base. `PrecioFinal` no viaja en el XML: tras `Parse`, vuelva a indicarlo con el precio pactado que conserve la aplicación (prevalece siempre) o, solo si no lo conserva, llame a `DeclararPreciosFinales()`, que lo reconstruye a partir del XML, antes de validar, recalcular o hacer la nota de crédito.
- `Validar` (y `Preparar`) rechaza importes incoherentes aunque no vengan de `Calcular`; para una nota de crédito, usa `DesdeFactura` o `AsociarFactura` para que se contraste con su factura.
- El secuencial lo asigna y lo guarda la aplicación, no el SDK.
- Fecha de emisión con `comprobante.Hoy()` (hora de Ecuador).
- Nunca envíes a producción (`comprobante.Produccion`) sin que la persona responsable lo pida expresamente; desarrolla contra `sritest` o el ambiente de pruebas.
- No pongas contraseñas, certificados `.p12` ni credenciales SMTP en el código, en los registros ni en el repositorio.
- Los códigos de país son los de `comprobante.Paises` (tabla del SRI), no ISO 3166.
- `valorDevolucionIva` debe omitirse o ser cero: la devolución automática de IVA (DIG) no está implementada y el SDK rechaza valores positivos.
- Para pruebas automáticas use `sritest.Nuevo()` y `sritest.CertificadoDePrueba()`.

## Si vas a MODIFICAR el SDK

Las pruebas de `internal/interop` contrastan la firma con `xmlsec1` y la canonicalización y el XSD con `xmllint`; se omiten si no están instalados (`SRI_INTEROP=1` las hace obligatorias, como en el CI).

```sh
go build ./... && go vet ./... && go test -race ./...
gofmt -l <directorios del proyecto>  # no ejecutes gofmt sobre "." si existe research/
```

- Código, comentarios y documentación en español; los nombres siguen las etiquetas XML del SRI (`InfoTributaria`, `RazonSocial`).
- La especificación es la ficha técnica del SRI (versión citada en `docs/fuentes.md`); cita la sección o el anexo en los comentarios de cada regla.
- Dependencias mínimas: antes de añadir un módulo, justifícalo en `docs/decisiones.md` y anota su licencia en `NOTICE` y `docs/fuentes.md`. Sin cgo.
- Cada regla de validación tiene un identificador estable (`Hallazgo.Regla`) y una prueba. Los datos que caducan se generan con un script y se anotan en `docs/actualizar-catalogos.md`.
- Lo que se afirma en `docs/capacidades.md` y `docs/pruebas-sri.md` debe distinguir lo probado contra el SRI real de lo probado solo en local.
- No incluyas en el repositorio certificados, contraseñas, `.env`, comprobantes reales ni datos de contribuyentes; los ejemplos usan datos ficticios o los que publica la ficha técnica.
- Trabaja en ramas `feat/`, `fix/` o `chore/` y fusiona con el CI en verde.

## Dónde está cada cosa

| Tema | Archivo |
|---|---|
| Cómo integrar | `docs/guia-de-integracion.md` |
| Ejemplo ejecutable | `ejemplos/emision/main.go`, `example_test.go` |
| Qué está implementado y sus límites | `docs/capacidades.md` |
| Qué falta y cómo continuarlo | `docs/pendientes.md` |
| Protocolo del SRI, independiente del lenguaje | `docs/referencia/`, `vectores/` |
| Normas para contribuir | `CONTRIBUTING.md` |
| Qué se ha probado contra el SRI | `docs/pruebas-sri.md` |
| Decisiones de diseño | `docs/decisiones.md` |
| Datos que caducan y cómo actualizarlos | `docs/actualizar-catalogos.md` |
| Devolución automática de IVA (DIG), no implementada | `docs/devolucion-iva-dig.md` |
| Fuentes y licencias | `docs/fuentes.md`, `NOTICE` |
| Auditorías de v0.1.0 y de la candidata a 0.2.0, y certificación de 0.2.0 | `docs/auditoria-v0.1.0.md`, `docs/auditoria-v0.2.0.md`, `docs/certificacion-v0.2.0.md` |
| Revisar el SDK sin contexto previo | `docs/revision-independiente.md` |

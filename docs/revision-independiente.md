# Guía para una revisión independiente

Este documento orienta a quien revise el SDK sin contexto previo: qué hay, cómo reproducir cada comprobación y qué evidencia existe. No pretende decir dónde buscar problemas.

## Objeto

- **Versión publicada anterior:** `v0.1.0` (commit `395b6d0`), auditada ya una vez; resultados en [`auditoria-v0.1.0.md`](auditoria-v0.1.0.md).
- **Candidata a 0.2.0:** la rama `main` en el commit indicado por quien encarga la revisión. No hay etiqueta `v0.2.0` todavía. `git diff v0.1.0..main` muestra todo lo que cambió; los cambios de cada hallazgo están enlazados por commit en `auditoria-v0.1.0.md`.

## Reproducir

Solo se necesita Go 1.26 o superior; nada requiere conexión con el SRI ni un certificado real.

```sh
go vet ./... && go test -race -count=1 ./...     # pruebas, incluidas las regresiones de la auditoría
CGO_ENABLED=0 go build ./...
go run ./ejemplos/emision                         # flujo completo contra el simulador
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go mod verify

# Contraste con implementaciones independientes (xmlsec1 1.2 o 1.3, libxml2):
sudo apt-get install xmlsec1 libxml2-utils
SRI_INTEROP=1 go test -count=1 -v ./internal/interop

# Fuzzing del analizador XML:
go test -run='^$' -fuzz=FuzzC14NIdempotente ./internal/xmldom
go test -run='^$' -fuzz=FuzzNormalizacionDeAtributos ./internal/xmldom
```

El CI ([`.github/workflows/ci.yml`](../.github/workflows/ci.yml)) ejecuta lo mismo en cada cambio.

## Evidencia disponible

| Qué | Dónde |
|---|---|
| Hallazgos de la primera auditoría, su resolución, prueba y commit | [`auditoria-v0.1.0.md`](auditoria-v0.1.0.md) |
| Pruebas de regresión de esos hallazgos | `*/importes_test.go`, `factura/propiedad_test.go`, `factura/certificacion_test.go`, `retencion/sustento_test.go`, `retencion/calcular_redondeo_test.go`, `notacredito/origen_test.go`, `autorizacion_test.go`, `robustez_test.go`, `xades/huella_test.go`, `internal/xmldom/atributos_test.go`, `ride/vistaprevia_test.go` |
| Contraste con xmlsec1 y libxml2 | [`internal/interop`](../internal/interop/interop_test.go) |
| Vectores independientes del lenguaje | [`vectores/`](../vectores/README.md) |
| Resultados contra el SRI real (sanitizados) | [`pruebas-sri.md`](pruebas-sri.md) y [`certificacion-v0.2.0.md`](certificacion-v0.2.0.md) |
| Código de las rondas contra el SRI | [`internal/qa`](../internal/qa/main.go), [`internal/qa/certificacion`](../internal/qa/certificacion/main.go) (requieren un emisor habilitado y su certificado) |
| Qué está implementado, con qué evidencia y qué límites | [`capacidades.md`](capacidades.md), [`pendientes.md`](pendientes.md) |
| Decisiones de diseño y su motivo | [`decisiones.md`](decisiones.md) |

## Lo que no está en el repositorio

- **Certificados, contraseñas y datos del emisor** de las pruebas reales, y los XML y respuestas completos del SRI, que contienen datos tributarios. Los documentos públicos resumen sus resultados.
- **El informe original de la primera auditoría.** Su contenido (hallazgos, severidad, evidencia y resolución) está en `auditoria-v0.1.0.md`.

## Decisiones deliberadas

Estas diferencias con lo que acepta el SRI o con otras implementaciones son decisiones documentadas, no omisiones; la revisión puede cuestionarlas igualmente:

- `iva.valor` rechaza un centavo de diferencia en el IVA de una línea que el SRI autoriza (D17).
- El total de IVA se acepta sobre la base acumulada o como suma de las líneas (D5).
- No se exige el dígito verificador del RUC (D6); se ajusta en memoria un XSD publicado (D7).
- `valorDevolucionIva` mayor que cero se rechaza (D14).
- Un detalle sin IVA y las cotas de la retención frente a su sustento son advertencias, no errores (la segunda, error con `SustentoEstricto`).
- `Decimal.Digitos` cuenta los ceros finales, más estricto que el XSD al leer documentos de terceros.

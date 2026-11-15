# Política de versiones

El SDK sigue [SemVer](https://semver.org/lang/es/) y publica cada versión como un tag `vX.Y.Z` con su *release* en GitHub y su entrada en [`CHANGELOG.md`](../CHANGELOG.md). Una versión publicada no se modifica: cualquier cambio posterior sale en una versión nueva.

## Qué significa cada número

**Mientras la versión sea 0.x (situación actual):**

| Cambio | Versión |
|---|---|
| Corrección de errores, actualización de catálogos o de documentación, sin cambiar la API | parche: `0.1.0 → 0.1.1` |
| Funcionalidad nueva, o cualquier cambio incompatible de la API | menor: `0.1.x → 0.2.0` |

En 0.x una versión menor **puede romper la compatibilidad**. Cada cambio incompatible se anuncia en el `CHANGELOG.md` bajo «Cambios incompatibles», con la forma de adaptar el código. Fije la versión en su `go.mod` y lea las notas antes de actualizar.

**A partir de la 1.0.0:** los cambios incompatibles solo irán en una versión mayor.

## Pre-releases

Una versión marcada como *pre-release* en GitHub está probada contra el ambiente de pruebas del SRI, pero no se declara apta para producción. Las versiones candidatas llevarán sufijo (`v0.2.0-rc.1`).

## Cambios del SRI

Cuando el SRI publique una nueva ficha técnica, XSD o catálogo, la actualización correspondiente saldrá como parche si no cambia la API, o como versión menor si la cambia. El `CHANGELOG.md` indicará siempre la versión de la ficha técnica a la que corresponde cada versión del SDK. Ver [`actualizar-catalogos.md`](actualizar-catalogos.md).

## Qué se considera API

Los paquetes públicos del módulo, los identificadores de las reglas de validación (`Hallazgo.Regla`) y el formato de los XML que el SDK genera. No forman parte de la API: los paquetes bajo `internal/`, el texto de los mensajes de error, el aspecto del RIDE y las herramientas de `cmd/` y `ejemplos/`.

## Criterios para la 1.0.0

Orientativos, no un compromiso de fecha:

- Uso real en producción informado por más de un emisor.
- Firma comprobada con certificados de varias entidades de certificación.
- API estable durante al menos dos versiones menores.
- Los pendientes que afecten a la corrección de los importes, resueltos o protegidos (como hoy `valorDevolucionIva`).

## Cómo se publica una versión

1. Todo el cambio está en `main`, con el CI en verde.
2. `CHANGELOG.md` tiene la entrada de la versión, con requisitos, cambios, cambios incompatibles y limitaciones.
3. `go vet`, `go test -race`, `govulncheck` y la revisión de secretos pasan sobre el commit exacto.
4. Se crea el tag anotado sobre ese commit y el *release* en GitHub con las notas del `CHANGELOG.md`.
5. Se comprueba, en un entorno limpio, que `go get github.com/OsirisRoman/sri-ecuador-go@vX.Y.Z` instala y compila.

Los tags no se mueven ni se borran: el proxy de módulos de Go conserva cada versión publicada.

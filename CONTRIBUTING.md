# Cómo contribuir

Gracias por su interés. Este proyecto quiere ser una biblioteca útil y, a la vez, una referencia abierta sobre la facturación electrónica del SRI. Toda ayuda cuenta: corregir un error, probar con un certificado de otra entidad, completar un pendiente, mejorar una explicación o portar las ideas a otro lenguaje.

No hace falta ser experto en tributación ni en criptografía. Lo que sí se pide es **no afirmar más de lo que se ha comprobado** y **no exponer datos privados**.

## Formas de ayudar

| Quiero… | Empiece por |
|---|---|
| Reportar un error | Un *issue* con la plantilla «Error» |
| Contar cómo me fue contra el SRI (otra entidad de certificación, producción, un caso no probado) | La plantilla «Informe de interoperabilidad» |
| Implementar algo que falta | [`docs/pendientes.md`](docs/pendientes.md): cada entrada dice qué falta y cuándo se considera terminada |
| Actualizar el SDK tras un cambio del SRI | [`docs/actualizar-catalogos.md`](docs/actualizar-catalogos.md) |
| Mejorar la documentación | Un *pull request* directo |
| Hacer un SDK en otro lenguaje | [`docs/referencia/`](docs/referencia/README.md) y [`vectores/`](vectores/README.md); avísenos y lo enlazamos |
| Proponer un cambio grande | Un *issue* antes de escribir código |

## Entorno de desarrollo

Solo se necesita **Go 1.26 o superior**. No hacen falta cgo, Java, bases de datos ni un certificado: las pruebas generan certificados desechables y usan un simulador del SRI.

```sh
git clone https://github.com/OsirisRoman/sri-ecuador-go
cd sri-ecuador-go
go test ./...                 # todo, sin conexión
go run ./ejemplos/emision     # flujo completo contra el simulador
```

Antes de enviar un cambio:

```sh
gofmt -l .                    # no debe listar nada
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build ./...
```

La integración continua ejecuta lo mismo con dos versiones de Go y, además:

- **Seguridad:** `go mod verify` y `govulncheck` (también cada semana, aunque no haya cambios).
- **Interoperabilidad:** la firma contra `xmlsec1` y la canonicalización y el XSD contra `xmllint` (libxml2), con `SRI_INTEROP=1 go test ./internal/interop`, más un fuzzing breve del analizador XML. En local, esas pruebas se omiten si no tiene las herramientas (`apt-get install xmlsec1 libxml2-utils`).

Las acciones del CI se fijan por SHA, con la versión en un comentario. Para actualizar una, tome el SHA de la etiqueta nueva (`gh api repos/actions/checkout/commits/vX.Y.Z --jq .sha`) y cambie SHA y comentario a la vez.

### Probar contra el SRI (opcional)

Solo si tiene un RUC habilitado en el **ambiente de pruebas** y un certificado de firma. Copie `.env.example` a `.env` (está excluido de Git), complete sus datos y use `go run ./cmd/sri`. Las pruebas consumen secuenciales de su ambiente de pruebas. **Nunca use producción para probar el SDK.**

## Normas del código

- **Idioma:** código, comentarios y documentación en español. Los nombres siguen las etiquetas XML del SRI (`InfoTributaria`, `RazonSocial`), para leer el código junto a la ficha técnica.
- **Fuente de verdad:** la ficha técnica y los XSD del SRI. Cite la sección, tabla o anexo en el comentario de cada regla.
- **Importes:** siempre `decimal.Decimal`, nunca `float64`.
- **Validaciones:** cada regla lleva un identificador estable (`Hallazgo.Regla`) y una prueba. Si la regla depende de un dato que caduca, que sea una advertencia.
- **Dependencias:** las mínimas, sin cgo en el núcleo. Si propone una, explique por qué en [`docs/decisiones.md`](docs/decisiones.md) y anote su licencia en `NOTICE` y [`docs/fuentes.md`](docs/fuentes.md). No se acepta código con licencias incompatibles con Apache 2.0 (por ejemplo, GPL o LGPL).
- **Datos generados:** los catálogos se generan con un script desde la fuente oficial, no se transcriben a mano.
- **Compatibilidad:** hasta la versión 1.0 la API puede cambiar, pero explique cada cambio incompatible en `CHANGELOG.md`. Ver la [política de versiones](docs/versionado.md).

## Qué debe traer un *pull request*

1. Una rama (`feat/…`, `fix/…`, `docs/…`) a partir de `main`, con un solo propósito.
2. Pruebas del cambio. Para una regla nueva, un caso que la cumple y otro que la incumple.
3. La documentación al día: [`docs/capacidades.md`](docs/capacidades.md) si cambia lo que el SDK hace; [`docs/pendientes.md`](docs/pendientes.md) si completa o descubre un pendiente; [`docs/pruebas-sri.md`](docs/pruebas-sri.md) si aporta una prueba contra el SRI; `CHANGELOG.md`.
4. En la descripción: qué cambia, por qué, y **cómo se comprobó**, distinguiendo lo probado solo en local de lo confirmado por el SRI.

Los cambios se revisan cuando hay disponibilidad; este es un proyecto mantenido por voluntarios y no se garantizan plazos.

## Estados: cómo describir lo que se ha comprobado

Use siempre el vocabulario de [`docs/capacidades.md`](docs/capacidades.md):

- **Verificada en el SRI:** el ambiente de pruebas (o producción, indicándolo) respondió `AUTORIZADO`.
- **Probada localmente:** pasa el XSD oficial, el simulador o los vectores.
- **Parcial** o **no implementada**, diciendo qué falta.

`RECIBIDA` no es una autorización, y un comprobante que el simulador autoriza no está confirmado por el SRI.

## Aportar pruebas de interoperabilidad sin exponer datos

Los resultados contra el SRI son la contribución más valiosa y la que más cuidado exige.

**Nunca incluya** en un *issue*, un *pull request* o un mensaje de *commit*:

- El archivo `.p12`, su contraseña o cualquier clave privada.
- El archivo `.env` o credenciales (SMTP, portal del SRI).
- Comprobantes reales, claves de acceso reales, RUC, cédulas, razones sociales, direcciones o correos de personas o empresas.

**Sí es útil y seguro compartir:**

- El tipo y la versión del comprobante, el algoritmo de firma y la **entidad que emitió el certificado** (por ejemplo, «Security Data»), no el certificado.
- El ambiente (pruebas o producción) y la fecha.
- El estado que respondió el SRI y el **código y el texto de sus mensajes**.
- Un XML de ejemplo con **datos ficticios** que reproduzca el caso. Sustituya el RUC por `1790000000001`, los nombres por textos de ejemplo y regenere la clave de acceso.
- Para problemas de firma: el contenido de `X509IssuerName` que generó el SDK. Describe a la entidad de certificación, no a usted.

Si duda sobre si algo es sensible, no lo publique y pregunte primero en términos generales.

## Seguridad

Si encuentra una vulnerabilidad, no abra un *issue* público: siga [`SECURITY.md`](SECURITY.md).

## Licencia de las contribuciones

Al contribuir acepta que su aportación se distribuya bajo la **Apache License 2.0**, igual que el resto del proyecto (sección 5 de la licencia). Aporte solo código que haya escrito usted o que tenga derecho a aportar, e indique el origen y la licencia de cualquier material de terceros.

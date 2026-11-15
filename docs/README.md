# Documentación

La documentación oficial del SRI conserva la autoridad normativa; aquí se explica cómo usar el SDK, qué está comprobado y cómo funciona el protocolo.

## Usar el SDK

| Documento | Para qué |
|---|---|
| [Guía de integración](guia-de-integracion.md) | Flujo paso a paso, estados, reglas que no se deben romper, qué guardar, errores frecuentes |
| [Ejemplo de referencia](../ejemplos/emision/main.go) | Programa completo y comentado; `go run ./ejemplos/emision` |
| [Instrucciones para asistentes de IA](../AGENTS.md) | Lo esencial para integrar o modificar el SDK con ayuda de una IA |

## Estado, compatibilidad y pruebas

| Documento | Para qué |
|---|---|
| [Capacidades y estado](capacidades.md) | Qué hace cada parte, con qué respaldo y qué límites |
| [Pruebas contra el SRI](pruebas-sri.md) | Qué autorizó el SRI, en qué condiciones, y qué no se ha probado |
| [Notas de versión](../CHANGELOG.md) | Cambios de cada versión |
| [Política de versiones](versionado.md) | Qué puede cambiar entre versiones y cómo se publican |

## Lo que falta

| Documento | Para qué |
|---|---|
| [Pendientes y oportunidades de contribución](pendientes.md) | Qué falta, por qué, qué se sabe y cuándo se considera terminado |
| [Devolución automática de IVA (DIG)](devolucion-iva-dig.md) | Investigación completa de un servicio no implementado |

## Cómo funciona (independiente del lenguaje)

| Documento | Para qué |
|---|---|
| [Referencia técnica](referencia/README.md) | Clave de acceso, XML, cálculo, firma XAdES-BES y servicios web, para implementarlos en cualquier lenguaje |
| [Vectores de prueba](../vectores/README.md) | Datos reproducibles para comprobar una implementación |
| [Decisiones de diseño](decisiones.md) | Por qué este SDK es como es |

## Contribuir y mantener

| Documento | Para qué |
|---|---|
| [Cómo contribuir](../CONTRIBUTING.md) | Entorno, normas, *pull requests* e informes de interoperabilidad sin exponer datos |
| [Datos que caducan](actualizar-catalogos.md) | Qué revisar cuando el SRI publica cambios y cómo regenerar los catálogos |
| [Fuentes y licencias](fuentes.md) | Normativa, proyectos estudiados y dependencias |
| [Seguridad](../SECURITY.md) | Cómo reportar una vulnerabilidad |

# ADR 0001 — Arquitectura hexagonal y MVU

- **Estado:** borrador
- **Fecha:** 2026-11-18

## Contexto

`tui-editor` unifica exploración de archivos, Git completo y monitorización de contenedores Docker en una sola TUI Go. El plan de fases (F1–F15) y los boundaries descritos en `AGENTS.md` y `.agents/architecture.md` exigen una base que permita crecer por capas sin acoplar la interfaz a los detalles de I/O.

Los riesgos concretos son:

- La UI es la parte que más cambia; si importa adaptadores concretos, cada cambio de API externa (Moby, salida de Git) se propaga a la vista.
- La TUI debe seguir respondiendo mientras se ejecutan operaciones lentas (`git`, Docker, lectura de archivos grandes).
- El proyecto necesita tests rápidos sin Docker ni red, y tests de integración opt-in.

## Decisión

Adoptar **arquitectura hexagonal (puertos y adaptadores)** para el acceso a datos y **Elm Architecture (Model–Update–View)** con Bubble Tea v2 para la interfaz:

- `internal/domain/` contiene entidades, value objects y errores tipados puros; no conoce a nadie.
- `internal/ports/` declara las interfaces con `context.Context` que consume la UI.
- `internal/adapters/fs|git|docker` implementan esos puertos y traducen errores externos a errores de dominio.
- `internal/tui/` (modelo raíz en `app`, sub-modelos en `components`) depende de `domain`/`ports`, nunca de un adaptador concreto.
- `cmd/tui-editor/main.go` es el único composition root: construye configuración, logger, adaptadores y modelo, y arranca el programa.
- Toda operación externa se ejecuta en un `tea.Cmd` con `context.Context` y devuelve un mensaje tipado; `Update` y `View` no hacen I/O.

## Consecuencias

Positivas:

- Sustituir o simular un adaptador no toca la UI: los fakes de `internal/testutil/fakes/` permiten tests unitarios sin Docker, sin `git` y sin filesystem real.
- La dirección de dependencias queda verificable de forma mecánica en revisión de código.
- `Update`/`View` síncronos evitan bloqueos de la interfaz y simplifican el razonamiento sobre el estado.
- Los límites de recursos (previews, logs, paginación) viven en puertos y dominio, no repartidos por la vista.

Costes:

- Más paquetes y algo de cableado (interfaces, mensajes tipados, traductores de error) antes de ver la primera feature.
- Riesgo de interfaces especulativas: cada puerto debe nacer con un consumidor real y su fake, no antes.
- Los adaptadores requieren disciplina extra (timeouts, cancelación, límites de buffer) para no filtrar detalles.

## Referencias

- `AGENTS.md` — reglas duras y tabla de boundaries.
- `.agents/architecture.md` — capas, anti-patrones y flujo MVU.
- `.agents/testing.md` — estrategia de tests por capa.

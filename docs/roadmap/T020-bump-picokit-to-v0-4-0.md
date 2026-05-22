---
estado: Completed
tipo: task
---
# T020: Bump picokit de v0.2.0 a v0.4.0

**Contribuye a**: cerrar el desfase del consumidor con la librería; aprovechar coverage-spec v1.1 (auto-discovery) y la firma variadic de `autoupdate.New`.

**Dependencia externa** (no expresable como `blocked_by` por estar en otro repo): requiere que `picokit/O04-autoupdate-envdisable-optional-and-windows-fix/T003-release-v0-4-0` esté Completed y el tag `v0.4.0` publicado en GitHub.

## Contexto

`/home/shared/backscroll/go.mod` apunta a `github.com/pablontiv/picokit v0.2.0` mientras que picokit ya publicó v0.2.1, v0.3.0 y producirá v0.4.0 con O04.

El call-site `cmd/backscroll/main.go:9991` actual pasa los tres args (`"pablontiv/backscroll", "backscroll", "BACKSCROLL_AUTOUPDATE_DISABLE"`); seguirá compilando contra v0.4.0 sin modificación.

**Coordinación con O16 T004 (In Progress)**: backscroll está cerrando O16 T004 (replace bash con pkcov) contra v0.2.0. Conviene mergear T004 contra v0.2.0 antes del bump, o bumpear antes y dejar que T004 cierre contra v0.4.0 con auto-discovery v1.1. Cualquiera de los dos órdenes funciona; coordinar al implementar para evitar re-trabajo. La preferencia recomendada es **bumpear antes** para que T004 aproveche auto-discovery.

## Alcance

**In**:

1. Coordinar con quien tenga O16 T004 In Progress: confirmar orden de merge.
2. `cd /home/shared/backscroll && go get github.com/pablontiv/picokit@v0.4.0`.
3. `go mod tidy`.
4. Correr suite local:
   - `just check`
   - `just test`
   - El gate de coverage actual sigue siendo bash hasta que O16 T004 lo reemplace; pasará igual.
5. Push del commit con mensaje `chore(deps): bump picokit to v0.4.0`.

**Out**:
- No tocar el wiring de autoupdate ni cambiar `BACKSCROLL_AUTOUPDATE_DISABLE` — la firma variadic mantiene compatibilidad.
- No adelantar O16 T004/T005 — son tasks de ese roadmap.

## Estado inicial esperado

- `go.mod` declara `github.com/pablontiv/picokit v0.2.0`.
- Tag picokit `v0.4.0` publicado (precondición).
- O16 T004 In Progress (decidir orden de merge con quien la lleva).

## Criterios de Aceptación

- `go.mod` declara `github.com/pablontiv/picokit v0.4.0`.
- `go mod tidy` no introduce líneas extra.
- `just check && just test` exit 0.
- CI verde tras push.

## Fuente de verdad

- `/home/shared/backscroll/go.mod`
- `/home/shared/backscroll/go.sum`
- Conversación con quien tiene O16 T004 In Progress

---
tipo: outcome
---
# O16: Adoptar pkcov de picokit

Backscroll hoy tiene coverage total 86.2% y un gate de 85% via crossbeam, pero su `scripts/check-coverage.sh` (17 líneas bash) sólo valida el total — no aplica pisos por paquete. Eso permite que paquetes débiles se escondan tras el promedio ponderado. Concretamente: `internal/storage` está en 82.4% (bajo el floor) e `internal/models` no tiene tests, pero el gate de total no los señala.

Este outcome adopta el tooling compartido publicado en picokit (`O03-coverage-tooling`): backscroll empieza a consumir `pkcov check` desde su Justfile y pre-push hook, declara su `.coverage-floors.toml`, y cumple coverage-spec v1.0. Eso implica un pre-work modesto para subir los paquetes débiles a ≥85 antes de activar el piso (sin ese pre-work, el gate bloquearía todo push).

Resultado observable: `pkcov check` exit 0 sobre el árbol; per-package floors aplicados; pre-push detecta regresiones antes de que lleguen a CI; CLAUDE.md/README referencia `picokit/docs/coverage-spec.md`.

Invariantes preservadas:
- INV1: el threshold uniforme se mantiene en 85 (mismo número que el gate de CI actual; sólo cambia el mecanismo y la granularidad)
- INV2: el pre-work no relaja el contrato — sube los paquetes débiles, no agrega excepciones

Scope: pre-work + adopción mecánica. No se introducen features nuevas; no se cambia la lógica de backscroll. Depende cross-repo de picokit `O03-coverage-tooling` Completed con tag publicado.

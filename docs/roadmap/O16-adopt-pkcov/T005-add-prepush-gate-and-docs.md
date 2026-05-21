---
estado: Completed
tipo: task
---
# T005: Agregar pre-push gate + docs

**Outcome**: [O16 Adoptar pkcov de picokit](README.md)
**Contribuye a**: backscroll detecta regresiones antes de CI; declara conformance con coverage-spec v1.0

[[blocked_by:./T004-replace-bash-with-pkcov.md]]

## Preserva

- INV1 del outcome: threshold 85 aplicado antes del push
  - Verificar: regresión simulada bloquea `git push`

## Contexto

coverage-spec v1.0 sección 5 requiere que el repo ejecute `pkcov check` desde `.githooks/pre-push` cuando cambian archivos `*.go`. El patrón a imitar vive en `/home/shared/rootline/.githooks/pre-push`:

```bash
if git diff --name-only "$range" -- '*.go' 2>/dev/null | grep -q .; then
  echo "Checking coverage..."
  if ! just coverage-check > /tmp/backscroll-cov.log 2>&1; then
    cat /tmp/backscroll-cov.log
    echo "Coverage below threshold. Run: just coverage-check"
    exit 1
  fi
  echo "Coverage check passed."
fi
```

Después, actualizar README/CLAUDE.md (el que aplique) para referenciar `picokit/docs/coverage-spec.md` y declarar conformance.

## Alcance

**In**:

1. Editar `/home/shared/backscroll/.githooks/pre-push` (o crear si no existe) agregando el bloque condicional sobre `*.go` que llama `just coverage-check`.
2. Verificar que git hooks están enlazados (`git config core.hooksPath`); si no, instruir en CLAUDE.md o README.
3. Actualizar CLAUDE.md o README:
   - Sección de comandos: `just coverage` / `just coverage-check`
   - Sección de CI: referencia a `picokit/docs/coverage-spec.md`, declarar conformance con v1.0
4. Test: simular regresión (`git rm <test>.go && git commit && git push`) → debe bloquearse con mensaje de coverage; restaurar el archivo.

**Out**:
- No cambiar comportamiento de CI workflow (sólo agregar señal local).
- No documentar internals de pkcov (vive en picokit).

## Estado inicial esperado

- T004 completada: `just coverage-check` invoca pkcov y exit 0.
- `.githooks/pre-push` no menciona coverage.

## Criterios de Aceptación

- `.githooks/pre-push` incluye bloque de coverage gate condicional sobre `*.go`.
- Simulación de regresión bloquea push.
- Push de cambios docs-only (`*.md`) no dispara el gate.
- README o CLAUDE.md tiene sección de coverage que referencia el spec.
- Declaración explícita "backscroll cumple coverage-spec v1.0" en el doc.

## Fuente de verdad

- `/home/shared/backscroll/.githooks/pre-push`
- `/home/shared/backscroll/README.md` o `CLAUDE.md`
- `/home/shared/rootline/.githooks/pre-push` — patrón a imitar
- `/home/shared/picokit/docs/coverage-spec.md` — spec referenciada

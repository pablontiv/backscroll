---
tipo: outcome
---
# O15: Integrate picokit as a dependency

Cuando todas las tasks estén completadas, backscroll consumirá `github.com/pablontiv/picokit` como dependencia explícita, adoptando autoupdate (nuevo), output (reemplaza local con refactor `Format string→int`), hashfile (reemplaza `internal/sync.HashFile`, función que fue aportada a picokit en O01 de picokit), pathsec (adopción nueva en `internal/input_config/discover.go`), y eliminando `internal/diagnostics/` que es dead code.

Resultado esperado: codebase más pequeño, self-update vía staged async pattern disponible por primera vez en backscroll, una sola fuente upstream para utilidades genéricas (output formatting, file hashing, path safety), y la guarda anti-traversal aplicada al input discovery más expuesto.

Esto cierra el intent original de picokit (módulo compartido con `autoupdate` como componente principal) para backscroll, que era uno de los 3 consumidores previstos.

**Prerequisito externo:** picokit v0.1.1 (o superior) disponible vía `go get`.

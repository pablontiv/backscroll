---
id: T050
tipo: task
estado: Pending
titulo: Subcomando inputs validate
outcome: O13
dependencias: []
---

# T050 — Subcomando `inputs validate`

Añadir `backscroll inputs validate [--json]` al comando `inputs` en Go, mapeando la funcionalidad de `InputCommands::Validate` de v0 Rust.

## Alcance

En `cmd/backscroll/inputs.go`:
- Añadir `newInputsValidateCmd(stdout, stderr io.Writer) *cobra.Command`
- Flag `--json bool`
- Llamar `input_config.ActiveInputs(cfg.SessionDirs)` para cargar manifests
- Text output: `"Inputs valid: N active inputs"` o error descriptivo
- JSON output: `{"valid": true, "inputs": N}` o `{"valid": false, "error": "..."}`
- Registrar en `newInputsCmd` junto a los demás subcomandos

## Criterios de aceptación

- `backscroll inputs validate` imprime resumen de validación en texto
- `backscroll inputs validate --json` imprime JSON `{"valid": bool, ...}`
- Con manifests inválidos: sale con código no-zero y mensaje de error
- `go test ./cmd/backscroll/...` incluye test para este subcomando
- Coverage ≥85% mantenido

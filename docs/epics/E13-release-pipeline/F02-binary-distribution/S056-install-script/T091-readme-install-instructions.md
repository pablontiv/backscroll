---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T091: Update README with installation instructions

**Story**: [S056 Install Script](README.md)
**Contribuye a**: README tiene secciones claras de instalacion

[[blocks:T090-install-sh]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

El README actual tiene una seccion de instalacion basica. Necesita actualizarse con tres metodos: install script (recomendado), descarga directa de binario, y cargo install (para desarrolladores).

## Alcance

**In**:
1. Reescribir seccion de instalacion en README.md
2. Documentar tres metodos: script, binario directo, cargo
3. Agregar badges de version y CI status si no existen

**Out**: Documentacion de uso (ya existe)

## Estado inicial esperado

- README.md con seccion de instalacion existente
- install.sh creado (T090)

## Criterios de Aceptacion

- README tiene seccion "Installation" con 3 metodos documentados
- Cada metodo tiene un bloque de codigo copiable
- Script method es el primero (recomendado)

## Fuente de verdad

- `README.md`

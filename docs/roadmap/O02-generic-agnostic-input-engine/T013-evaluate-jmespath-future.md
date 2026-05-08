---
estado: On Hold
tipo: task
---
# T013: Evaluate JMESPath as future advanced mapping language

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: Decisión futura posterior al MVP

## Preserva

- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: esta task no bloquea T001-T012 y no agrega dependencia por sí misma.

## Contexto

JMESPath puede ofrecer mapping más expresivo que JSONPath + operadores declarativos simples, pero también agrega complejidad, superficie de errores y documentación. El MVP debe usar JSONPath y operadores cerrados; esta task queda pendiente.

## Alcance

**In**:
1. Comparar JMESPath contra JSONPath + operadores declarativos simples.
2. Evaluar crates Rust disponibles y mantenimiento.
3. Evaluar casos que no cubra el MVP.
4. Documentar decisión: adoptar, rechazar o posponer.

**Out**:
- Implementar JMESPath.
- Agregar dependencia a `Cargo.toml`.
- Bloquear el MVP.

## Estado inicial esperado

- El engine MVP usa JSONPath (`serde_json_path`) y operadores propios.
- No hay dependencia `jmespath`.

## Criterios de Aceptación

- Existe nota de decisión con pros/contras y ejemplos concretos.
- Si se recomienda adopción, hay una propuesta de task futura separada.
- Si no se recomienda, se documenta por qué JSONPath + operadores basta.

## Fuente de verdad

- `Cargo.toml`
- `docs/roadmap/O02-generic-agnostic-input-engine/README.md`
- Evidencia externa de Vector/Redpanda/OTel y crates Rust evaluadas.

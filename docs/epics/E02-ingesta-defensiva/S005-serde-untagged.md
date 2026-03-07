# S005: Serde Untagged (Parser Defensivo)

**Estado:** Pending
**ID:** S005
**Parent:** E02

Implementar la lógica de deserialización para manejar los esquemas variables de los logs de Claude Code.

## Tasks

- [ ] `T005`: Definir el enum `MessageContent` con `#[serde(untagged)]`.
- [ ] `T006`: Implementar el parser para leer archivos JSONL de forma segura.
- [ ] `T007`: Manejar casos de campos omitidos o de tipo inesperado mediante `serde(default)`.

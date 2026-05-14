---
id: T039
tipo: task
estado: Pending
titulo: Evaluar e integrar sqlite-vec (pure Go vs CGO)
outcome: O10
dependencias: [T038]
---

# T039 — Evaluar e integrar sqlite-vec

sqlite-vec es la extensión SQLite para búsqueda de vectores densos. Decisión
clave: `asg017/sqlite-vec-go-static` (pure Go, incluye la extensión compilada
como static lib) vs binding CGO puro.

## Evaluación requerida

| Opción | Pros | Contras |
|---|---|---|
| `asg017/sqlite-vec-go-static` | Diseñado para pure-Go SQLite drivers | Puede requerir CGO para compilar la extensión |
| Extensión nativa (.so) cargada en runtime | Más flexible | CGO + .so en sistema, no cross-compilable |
| Emulación pure Go (HNSW manual) | Sin CGO, sin dependencias | Alto esfuerzo, menor performance |

**Criterio**: si `sqlite-vec-go-static` funciona con `modernc.org/sqlite` sin CGO, usar.
Si no, documentar la limitación y usar emulación pure Go para la PoC de O10.

## Alcance

- Probar `asg017/sqlite-vec-go-static` con `modernc.org/sqlite`: crear tabla `vec_embeddings`,
  insertar vector, hacer similarity search → medir si funciona sin CGO
- Documentar resultado en commit message o ADR
- Si funciona: integrar y proceder con T040
- Si falla: implementar `VecIndex` simple en Go (linear scan, cosine similarity)
  como fallback funcional (peor performance, mismo API)

## Criterios de aceptación

- `go build ./...` sin CGO o con la decisión documentada
- La extensión carga correctamente en tests
- `SELECT * FROM vec_search(vec_embeddings, ?, 10)` retorna resultados
- Decisión documentada en commit o ADR

## Notas

- Esta task desbloquea T040 y T041 — la decisión aquí determina el approach de ambas

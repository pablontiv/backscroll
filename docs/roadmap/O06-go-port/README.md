---
tipo: outcome
estado: Completed
---
# Port a Go

Reescribir backscroll en Go en la rama `main`, manteniendo la rama `v0` con el código Rust congelado.

Stack: cobra, go-toml/v2, goldmark, modernc.org/sqlite (puro Go, sin CGO), stdlib testing — alineado con roadmapctl y rootline. Paridad completa con los 13 comandos actuales, sin el stack de embeddings (ONNX/sqlite-vec), que no está activo en producción.

El resultado debe ser un binary cross-compilable trivialmente (`GOOS=linux go build`) con builds significativamente más rápidos que el pipeline Rust actual.

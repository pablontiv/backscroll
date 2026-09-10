---
tipo: adr
estado: accepted
fecha: '2026-09-10'
contexto: 'La conjunción léxica pierde registros ante términos adicionales; las paráfrasis sin vocabulario compartido no se resuelven eliminando términos.'
decision: 'Ofrecer --relax explícito: búsqueda estricta primero, eliminación por menor IDF tras cero filas, núcleo mínimo de dos términos no protegidos, +término y frases protegidos, filtros invariantes y procedencia obligatoria.'
alternativas: 'OR global y ampliación de ámbito: descartados por pérdida de precisión; expansión semántica y cambio del valor predeterminado: fuera del alcance; relajar frases protegidas: contradice la intención aprobada.'
consecuencias: 'La salida predeterminada permanece estable; el modo explícito limita unidades y conserva procedencia dentro del presupuesto; la recuperación depende del solapamiento léxico y de la frecuencia documental.'
---
# 0009. Relajar lexico solo con opt in

## Contexto

La conjunción léxica pierde registros ante términos adicionales; las paráfrasis sin vocabulario compartido no se resuelven eliminando términos.

## Decisión

Ofrecer --relax explícito: búsqueda estricta primero, eliminación por menor IDF tras cero filas, núcleo mínimo de dos términos no protegidos, +término y frases protegidos, filtros invariantes y procedencia obligatoria.

## Alternativas descartadas

OR global y ampliación de ámbito: descartados por pérdida de precisión; expansión semántica y cambio del valor predeterminado: fuera del alcance; relajar frases protegidas: contradice la intención aprobada.

## Consecuencias

La salida predeterminada permanece estable; el modo explícito limita unidades y conserva procedencia dentro del presupuesto; la recuperación depende del solapamiento léxico y de la frecuencia documental.

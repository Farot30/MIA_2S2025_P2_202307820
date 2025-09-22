# **Informe de Impacto - Sistema de Manejo e Implementación de Archivos**

Version: 1.0

Autor: Ottoniel Vásquez

Fecha: Septiembre 2025

---

## 1. **Resumen**

Este documento evalúa el impacto potencial del Sistema de Manejo e Implementación de Archivos, un entorno educativo y de simulación para administradores de discos virtuales (.mia) particiones y sistema de archivos tipo EXT2 en su contexto de aplicación. Se analizan impactos técnicos, operativos, económicos, sociales y riesgos, y se proponen métricas y acciones para maximizar beneficioes y mitigar riesgos.

## 2. **Contexto y objetivos del proyecto**

El sistema de Manejo e Implementación de Archivos nació como herramienta de apoyo en asignaturas de Sistemas Operativos y Administración de Sistemas. Permite a estudiantes crear discos virtuales, manipular particiones, formatear y operar un FS simulado, gestionar usuarios y generar reportes. Los objetivos de impacto son:

* Mejorar la compresión práctica de estructuras de disco y sistemas de archivos.
* Reducir la barrera técnica para analizar experimentos seguros (sin modificar discos reales).
* Acelerar la evaluación y retroalimentación docente mediante reportes automáticos.

## 3. **Alcance del análisis**
El análisis abarca: implementación en entornos educativos (laboratorios y tareas remotas), uso por parte de estudiantes y docentes, integración con procesos de evaluación y mantenimiento por el equipo de desarrollo.

## 4. **Impacto técnico**
### 4.1. **Mejora en la eficiencia del aprendizaje**

* El acceso inmediato a operaciones de disco (mkdisk, fdisk, mkfs, mkfile, etc.) reduce el tiempo necesario para que un estudiante comprenda conceptos teóricos y procedimentales.

### 4.2. **Entorno seguro para experimentación**

* Al aislar operaciones dentro de archivos .mia, se elimina el riesgo de dañar sistemas reales, lo que permite experimentación libre y más agresiva por parte de estudiantes.

### 4.3. **Reproducibilidad y trazabilidad**

* Los comandos y los reportes (rep) permiten reproducir escenarios y revisar decisiones (mbr, bitmap, árbol de directorios). Esto facilita la corrección de prácticas y el análisis de errores.

## 5. **Impacto operativo**
### 5.1. **Reducción de carga administrativa docente**

* Automatización de reportes y la capacidad de auditar el estado de cada práctica reduce el tiempo de corrección manual. Esto puede liberar entre 20% y 40% del tiempo dedicado a evaluación práctica.

### 5.2. **Mejora en la calidad de retroalimentación**

* Reportes visuales y logs permiten dar retroalimentación puntual, aumentando la calidad y rapidez de la evaluación.

## 6. **Impacto económico**
### 6.1. **Reducción de costos directos**

* Evita la necesidad de hardware adicional para prácticas sobre discos reales, ahorrando en gastos de reemplazo o recuperación ante errores de estudiantes.

### 6.2. **Costos de implementación**

* Gastos asociados a despliegue (servidor para backend si se desea centralizar), licencias y tiempo del personal docente para instalación y capacitación. Para una implementación básica en laboratorios existentes, los costos monetarios son bajos; el mayor coste es el tiempo humano inicial.

### 6.3. **Retorno de la inversión (ROI)**

* ROI positivo a medio plazo si se considera reducción de tiempo de corrección y disminución de incidentes en equipos. Recomendado medir ROI en un semestre académico registrando horas dedicadas a corrección antes y después de la implementación.

## 7. **Impacto académico y formativo**

* Aumento del aprendizaje práctico y habilidades técnicas en: estructuras de disco, gestión de particiones, inodos, bitmaps, derechos de usuario y herramientas de diagnóstico.

* Facilita la realización de laboratorios remotos y prácticas autocalificables o semi‑autocalificables.

* Potencia proyectos de investigación estudiantil que requieran manipular sistemas de archivos sin riesgo físico.

## 8. **Impacto social y ético**

* Fomenta prácticas seguras: al evitar tocar discos reales se reduce el riesgo de pérdida de datos de usuarios.

* La herramienta debe acompañarse con políticas de uso y protección de datos en caso de posibles datos reales cargados por estudiantes (asesorar para anonimización).

## 9. **Riesgos identificados y medidas de mitigación**

1. **Riesgo: Datos sensibles cargados por estudiantes**

* Mitigación: Añadir advertencias y políticas; proporcionar datos de ejemplo; borrar automáticamente imágenes .mia tras evaluación o mantener repositorios aislados.

2. **Riesgo: Fallos en la generación de reportes (Graphviz)**

* Mitigación: Incluir validaciones previas; ofrecer reportes alternativos en texto; cache de imágenes generadas.

3. **Riesgo: Errores de interpretación por parte de estudiantes**

* Mitigación: Incluir guías paso a paso, ejemplos resueltos y ejercicios mínimos obligatorios.

4. **Riesgo: Sobrecarga del servidor en despliegues centralizados**

* Mitigación: Escalar horizontalmente, establecer límites de uso por usuario/sesión, usar colas para generación de reportes.

## 10. **Métricas sugeridas para medir impacto**

* Tiempo medio por práctica antes/después (horas).

* Tasa de éxito en tareas (porcentaje de prácticas correctamente ejecutadas).

* Horas de corrección por práctica (docente).

* Número de incidentes en equipos físicos atribuibles a prácticas.

* Uso de la herramienta (logins, comandos ejecutados, reportes generados).

Recoger métricas durante al menos un semestre permitirá cuantificar ganancias reales.

## 11. ****Recomendaciones**

1. Documentación y tutoriales: agregar guías paso a paso y videos cortos.

2. Integración con plataformas de gestión académica (LMS) para facilitar entrega y corrección.

3. Automatizar la limpieza periódica de archivos .mia y reportes viejos.

4. Medir métricas iniciales (línea base) y repetir mediciones tras 1 semestre.

## 12. **Conclusión**

El Sistema Manejo e Implementación de Archivos presenta un impacto mayoritariamente positivo en contextos educativos: mejora la eficiencia del aprendizaje práctico, reduce riesgos sobre hardware real y facilita la labor docente mediante reportes y trazabilidad. Los costos de implementación son reducidos frente a los beneficios esperados; la principal inversión es tiempo en documentación y despliegue inicial. Con medidas de mitigación a riesgos y métricas claras de evaluación, el proyecto puede consolidarse como herramienta estándar en asignaturas de Sistemas Operativos y Administración de Sistemas.
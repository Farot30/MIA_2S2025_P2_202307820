# **Plan de Mantenimiento**

## **1. Introducción **
Este documento propone un plan de mantenimiento y evolución para el Sistema de Manejo e Implementación de Archivos, con el objetivo de asegurar su disponibilidad, seugirdad, calidad y evolución continua a lo largo del tiempo. Cubre actividades de mantenimiento preventivo y correctivo, actualizaciones de software, gestión de incidencias, monitorización, copias de seguridad, pruebas, documentación y formación.

## **2. Objetivos del plan**

* Mantener la plataforma operativa y segura.

* Garantizar la integridad y disponibilidad de los archivos .mia y reportes.

* Facilitar la incorporación de mejoras y evoluciones con riesgo controlado.

* Proveer un proceso claro de soporte y respuesta ante incidencias.

## **3. Alcance**

* Aplica al frontend, backend (API en Go), utilidades de generación de reportes (Graphviz), scripts de despliegue, repositorio de código, infraestructura que lo aloje (servidor local o VM en nube) y documentación asociada.

## **4. Roles y responsabilidades**

* Responsable de Producto (RP): Prioriza cambios, roadmap y coordina con stakeholders (docencia).

* Equipo de Desarrollo (DEV): Implementa mejoras, corrige bugs, mantiene CI/CD y pruebas automatizadas.

* Administrador de Sistemas (SysAdmin): Gestiona servidores, backups, monitorización y despliegues.

* Soporte / Helpdesk: Primer nivel de atención para estudiantes y docentes; registra y prioriza incidencias.

* QA / Tester: Diseña y ejecuta pruebas (unitarias, integración, regresión) antes de cada release.

## **5. Tipos de mantenimiento**
### **5.1. Mantenimiento preventivo**

* Actualizaciones regulares de dependencias (mensual o trimestral según criticidad).

* Revisiones periódicas (mensual) de logs, espacio en disco y uso de CPU/RAM.

* Pruebas de restauración de backups (trimestral) para validar integridad.

* Revisión y actualización de documentación y guías (cada semestre o tras cambios relevantes).

### **5.2. Mantenimiento correctivo**

* Corrección de bugs críticos (SLA 48 horas) y no críticos (SLA 2 semanas).

* Parcheo de vulnerabilidades de seguridad (SLA: 72 horas para vulnerabilidades críticas).

* Procedimiento de rollback documentado para releases con regresiones.

### **5.3. Mantenimiento evolutivo**

* Implementación de nuevas funcionalidades según roadmap aprobado.

* Mejoras de rendimiento y experiencia de usuario (UX).

* Integraciones con LMS o herramientas académicas cuando se aprueben.

## **6. Política de actualizaciones y versiones**

* Versionado Semántico (SemVer): MAJOR.MINOR.PATCH

    - MAJOR: cambios incompatibles; requiere coordinación y ventanas de mantenimiento.

    - MINOR: nuevas funcionalidades compatibles.

    - PATCH: correcciones y hotfixes.

* Cadencia de releases: Releases menores cada 6–8 semanas; parches urgentes según necesidad.

* Ramas en Git: main para releases, develop para integración, ramas feature/bugfix por tarea.

## **7. Backups y recuperación ante desastres**

* Alcance de backup: archivos .mia, base de datos (si aplica), configuraciones, registros importantes y reportes.

* Frecuencia: backups incrementales diarios, snapshot completo semanal.

* Retención: 30 días (mínimo) para backups incrementales y 6 meses para snapshots completos (ajustable según política institucional).

* Pruebas de restauración: trimestrales (ver 5.1).

* Almacenamiento: almacenamiento externo (NAS) o nube (S3/compatible) con cifrado en reposo.

## **9. Monitorización y alertas**

* Métricas a vigilar: uso de disco, CPU/RAM del backend, latencia de API, tasa de errores 5xx, colas de generación de reportes.

* Alertas críticas: espacio en disco < 10%, errores 5xx > 1% de requests, proceso de generación de rep colapsado.

* Escalamiento: soporte → sysadmin → equipo de desarrollo según gravedad.

## **10. Gestión de incidencias**

* Registro: todas las incidencias deben registrarse en un sistema (Jira/GitHub Issues/GitLab Issues).

* Clasificación: Crítico / Alto / Medio / Bajo.

* SLA sugeridos: Crítico: 4 h respuesta inicial, resolución 48 h. Alto: 8 h respuesta, resolución 5 días. Medio: 24 h respuesta, resolución 2 semanas. Bajo: 48 h respuesta, resolución 1 mes.

* Comunicación: canal dedicado (email/Slack) para notificaciones a docentes y usuarios afectados.

## **11. Pruebas y aseguramiento de calidad**

* Pruebas unitarias: cobertura mínima objetivo 70% para backend crítico.

* Pruebas de integración: validar flujos clave (mkdisk → fdisk → mount → mkfs → mkfile → rep).

* Pruebas de regresión: antes de cada release mayor/minor.

* Pruebas de carga (opcional): simular múltiples usuarios ejecutando comandos y generando reportes para identificar cuellos de botella.

## **17. Calendario inicial de actividades (primer año)**

* Mes 0–1: Despliegue en producción, documentación inicial y capacitación.

* Mes 1–3: Configuración de backups y monitorización; pruebas de restauración.

* Mes 3–6: Automatización CI/CD y cobertura de pruebas.

* Mes 6–12: Integración con LMS (fase piloto) y mejora del roadmap.

## **18. Checklists (rápido)**

### **18.1. Checklist diario (sysadmin)**

- Verificar uso de disco y alertas pendientes.

- Revisar logs críticos.

- Asegurar que no hay colas de reportes atascadas.

### **18.2. Checklist semanal (DEV/SysAdmin)**

- Revisar issues abiertos y priorizar parches.

- Ejecutar pruebas automáticas de integración en staging.

### **18.3. Checklist trimestral**

- Prueba de restauración de backups.

- Revisión de dependencias y actualización mayor si procede.

- Actualizar documentación y capacitar al personal si hay cambios.

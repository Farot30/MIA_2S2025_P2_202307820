package commandsP2

import (
	"fmt"
	"sort"
	"strings"
)

// montadas en memoria con su ID correspondiente
func MOUNTED(args []string) {
	// Este comando no acepta parámetros, pero verificamos si se enviaron
	if len(args) > 0 {
		fmt.Println("Advertencia: El comando 'mounted' no acepta parámetros. Se ignorarán los parámetros proporcionados.")
	}

	// Obtener el mutex para lectura segura
	mountManager.mutex.RLock()
	defer mountManager.mutex.RUnlock()

	// Verificar si hay particiones montadas
	if len(mountManager.mountedPartitions) == 0 {
		fmt.Println("No hay particiones montadas en el sistema")
		return
	}

	// Mostrar encabezado
	fmt.Println("=== PARTICIONES MONTADAS ===")
	fmt.Println()

	// Crear una lista de IDs para mostrar en formato simple
	var mountedIDs []string

	// Mostrar información detallada de cada partición montada
	fmt.Printf("%-8s | %-25s | %-15s | %-10s | %s\n", "ID", "DISCO", "PARTICIÓN", "TIPO", "ESTADO")
	fmt.Println(strings.Repeat("-", 75))

	// Ordenar por ID para mostrar consistentemente
	var sortedIDs []string
	for id := range mountManager.mountedPartitions {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		partition := mountManager.mountedPartitions[id]
		mountedIDs = append(mountedIDs, id)

		// Truncar path si es muy largo para mejor visualización
		displayPath := partition.Path
		if len(displayPath) > 25 {
			displayPath = "..." + displayPath[len(displayPath)-22:]
		}

		partitionType := partition.Type

		fmt.Printf("%-8s | %-25s | %-15s | %-10s | %s\n",
			id,
			displayPath,
			partition.Name,
			partitionType,
			"Montada")
	}

	fmt.Println(strings.Repeat("-", 75))
	fmt.Printf("Total de particiones montadas: %d\n", len(mountManager.mountedPartitions))

	// Mostrar lista simple de IDs como en la especificación
	fmt.Println()
	fmt.Print("IDs montados: ")
	fmt.Println(strings.Join(mountedIDs, ", "))
}

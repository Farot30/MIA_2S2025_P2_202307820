package commandsP2

import (
	structuresP2 "MIA_2S2025_P2_202307820/structures"

	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

// Mover esta definición al inicio del archivo, después de los imports
type LogicalSpace struct {
	EBRPos    int32
	DataStart int32
	DataEnd   int32
	Name      string
}

// FDISK implementa el comando para crear particiones en discos virtuales
func FDISK(args []string) {
	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetros requeridos básicos
	required := []string{"path", "name"}
	if err := utilitiesP2.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// Obtener parámetros
	sizeStr := params["size"]
	path := params["path"]
	name := params["name"]
	unit := strings.ToUpper(params["unit"])
	partType := strings.ToUpper(params["type"])
	fit := strings.ToUpper(params["fit"])
	deleteType := strings.ToUpper(params["delete"])
	addStr := params["add"]

	// Validar operación (crear, eliminar o modificar)
	isDelete := deleteType != ""
	isAdd := addStr != ""
	isCreate := !isDelete && !isAdd

	// Si es crear, el size es obligatorio
	if isCreate && sizeStr == "" {
		fmt.Println("Error: El parámetro 'size' es obligatorio para crear particiones")
		return
	}

	var sizeInBytes int32
	if sizeStr != "" {
		// Validar y convertir tamaño
		size, err := strconv.Atoi(sizeStr)
		if err != nil || size <= 0 {
			fmt.Println("Error: El tamaño debe ser un número positivo mayor que cero")
			return
		}

		// Aplicar unidades (por defecto Kilobytes según especificación)
		switch unit {
		case "B":
			sizeInBytes = int32(size)
		case "K", "":
			sizeInBytes = int32(size * 1024)
		case "M":
			sizeInBytes = int32(size * 1024 * 1024)
		default:
			fmt.Printf("Error: Unidad '%s' no válida. Use B para Bytes, K para Kilobytes o M para Megabytes\n", unit)
			return
		}
	}

	// Validar tipo de partición (por defecto Primaria)
	switch partType {
	case "P", "":
		partType = "P"
	case "E", "L":
		// Válidos
	default:
		fmt.Printf("Error: Tipo de partición '%s' no válido. Use P para Primaria, E para Extendida o L para Lógica\n", partType)
		return
	}

	// Validar fit (por defecto Worst Fit según especificación)
	var fitByte byte
	switch fit {
	case "BF":
		fitByte = 'B'
	case "FF":
		fitByte = 'F'
	case "WF", "":
		fitByte = 'W'
	default:
		fmt.Printf("Error: Ajuste '%s' no válido. Use BF (Best Fit), FF (First Fit) o WF (Worst Fit)\n", fit)
		return
	}

	// Validar deleteType si se proporciona
	if deleteType != "" && deleteType != "FAST" && deleteType != "FULL" {
		fmt.Printf("Error: Tipo de eliminación '%s' no válido. Use FAST o FULL\n", deleteType)
		return
	}

	// Validar y convertir add si se proporciona
	var add int32
	if addStr != "" {
		addInt, err := strconv.Atoi(addStr)
		if err != nil {
			fmt.Println("Error: El valor de 'add' debe ser un número entero")
			return
		}
		add = int32(addInt)
	}

	// Asegurar extensión .mia
	if filepath.Ext(path) != ".mia" {
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ".mia"
	}

	// Validar si el disco existe
	if !utilitiesP2.FileExists(path) {
		fmt.Printf("Error: El disco '%s' no existe\n", path)
		return
	}

	// Abrir el archivo del disco
	file, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		fmt.Printf("Error abriendo el disco: %s\n", err)
		return
	}
	defer file.Close()

	// Leer el MBR
	var mbr structuresP2.MBR
	if err := utilitiesP2.ReadStruct(file, 0, &mbr); err != nil {
		fmt.Printf("Error leyendo el MBR: %s\n", err)
		return
	}

	// Procesar según el tipo de operación
	if isDelete {
		deletePartition(&mbr, file, name, deleteType)
	} else if isAdd {
		modifyPartition(&mbr, file, name, add)
	} else {
		createPartition(&mbr, file, name, partType, sizeInBytes, fitByte)
	}
}

// createPartition crea una nueva partición
func createPartition(mbr *structuresP2.MBR, file *os.File, name, partType string, size int32, fit byte) {
	// Validar restricciones según el tipo de partición
	if err := validatePartitionConstraints(mbr, partType, name); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// Buscar espacio disponible según el algoritmo de ajuste
	start, err := findAvailableSpace(mbr, size, fit)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// Crear la partición según el tipo
	switch partType {
	case "P":
		createPrimaryPartition(mbr, file, name, start, size, fit)
	case "E":
		createExtendedPartition(mbr, file, name, start, size, fit)
	case "L":
		createLogicalPartition(mbr, file, name, start, size, fit)
	}

	showDiskStatus(file.Name())
}

// validatePartitionConstraints valida las restricciones de particiones
func validatePartitionConstraints(mbr *structuresP2.MBR, partType, name string) error {
	primaryAndExtendedCount := 0
	hasExtended := false

	// Contar particiones existentes y verificar nombres duplicados
	for _, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 {
			// Verificar nombre duplicado
			partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
			if partName == name {
				return fmt.Errorf("ya existe una partición con el nombre '%s'", name)
			}

			// Contar primarias y extendidas
			if partition.PartType == 'P' || partition.PartType == 'E' {
				primaryAndExtendedCount++
			}

			// Verificar si ya hay extendida
			if partition.PartType == 'E' {
				hasExtended = true
			}
		}
	}

	// Validar restricciones específicas
	switch partType {
	case "P", "E":
		if primaryAndExtendedCount >= 4 {
			return fmt.Errorf("no se pueden crear más de 4 particiones primarias y extendidas")
		}
		if partType == "E" && hasExtended {
			return fmt.Errorf("solo puede haber una partición extendida por disco")
		}
	case "L":
		if !hasExtended {
			return fmt.Errorf("no se puede crear una partición lógica sin una partición extendida")
		}
	}

	return nil
}

// findAvailableSpace busca espacio disponible según el algoritmo de ajuste
func findAvailableSpace(mbr *structuresP2.MBR, size int32, fit byte) (int32, error) {
	// Esta función implementaría la lógica de First Fit, Best Fit o Worst Fit
	// Por ahora, implementación básica

	diskSize := mbr.MbrTamano
	used := int32(0)

	// Calcular espacio usado
	for _, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 {
			used += partition.PartS
		}
	}

	available := diskSize - used - int32(unsafe.Sizeof(structuresP2.MBR{}))

	if available < size {
		return 0, fmt.Errorf("no hay suficiente espacio disponible. Requerido: %d bytes, Disponible: %d bytes", size, available)
	}

	// Retornar posición donde empezará la partición

	return int32(unsafe.Sizeof(structuresP2.MBR{})) + used, nil
}

// findAvailableSpace busca espacio disponible según el algoritmo de ajuste
func findAvailableSpace1(mbr *structuresP2.MBR, size int32, fit byte) (int32, error) {
	mbrSize := int32(unsafe.Sizeof(structuresP2.MBR{}))
	diskSize := mbr.MbrTamano

	// Crear lista de espacios ocupados (incluyendo el MBR)
	var occupiedSpaces []struct {
		Start int32
		End   int32
	}

	// Agregar el MBR como espacio ocupado
	occupiedSpaces = append(occupiedSpaces, struct {
		Start int32
		End   int32
	}{Start: 0, End: mbrSize})

	// Agregar todas las particiones existentes
	for _, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 {
			occupiedSpaces = append(occupiedSpaces, struct {
				Start int32
				End   int32
			}{
				Start: partition.PartStart,
				End:   partition.PartStart + partition.PartS,
			})
		}
	}

	// Ordenar espacios ocupados por posición de inicio
	for i := 0; i < len(occupiedSpaces); i++ {
		for j := i + 1; j < len(occupiedSpaces); j++ {
			if occupiedSpaces[i].Start > occupiedSpaces[j].Start {
				occupiedSpaces[i], occupiedSpaces[j] = occupiedSpaces[j], occupiedSpaces[i]
			}
		}
	}

	// Buscar espacios libres
	var freeSpaces []struct {
		Start int32
		Size  int32
	}

	// Verificar espacio antes de la primera partición (después del MBR)
	if len(occupiedSpaces) > 1 {
		firstPartitionStart := occupiedSpaces[1].Start
		if firstPartitionStart > mbrSize {
			gapSize := firstPartitionStart - mbrSize
			if gapSize >= size {
				freeSpaces = append(freeSpaces, struct {
					Start int32
					Size  int32
				}{Start: mbrSize, Size: gapSize})
			}
		}
	}

	// Verificar espacios entre particiones
	for i := 1; i < len(occupiedSpaces)-1; i++ {
		currentEnd := occupiedSpaces[i].End
		nextStart := occupiedSpaces[i+1].Start

		if nextStart > currentEnd {
			gapSize := nextStart - currentEnd
			if gapSize >= size {
				freeSpaces = append(freeSpaces, struct {
					Start int32
					Size  int32
				}{Start: currentEnd, Size: gapSize})
			}
		}
	}

	// Verificar espacio al final del disco
	if len(occupiedSpaces) > 0 {
		lastEnd := occupiedSpaces[len(occupiedSpaces)-1].End
		if diskSize > lastEnd {
			gapSize := diskSize - lastEnd
			if gapSize >= size {
				freeSpaces = append(freeSpaces, struct {
					Start int32
					Size  int32
				}{Start: lastEnd, Size: gapSize})
			}
		}
	} else {
		// No hay particiones, todo el espacio después del MBR está libre
		gapSize := diskSize - mbrSize
		if gapSize >= size {
			freeSpaces = append(freeSpaces, struct {
				Start int32
				Size  int32
			}{Start: mbrSize, Size: gapSize})
		}
	}

	// Si no hay espacios libres suficientes
	if len(freeSpaces) == 0 {
		totalUsed := int32(0)
		for _, partition := range mbr.MbrPartitions {
			if partition.PartS > 0 {
				totalUsed += partition.PartS
			}
		}
		available := diskSize - totalUsed - mbrSize
		return 0, fmt.Errorf("no hay suficiente espacio disponible. Requerido: %d bytes, Disponible: %d bytes", size, available)
	}

	// Aplicar algoritmo de ajuste
	switch fit {
	case 'F': // First Fit
		return freeSpaces[0].Start, nil

	case 'B': // Best Fit
		bestIndex := 0
		bestSize := freeSpaces[0].Size
		for i := 1; i < len(freeSpaces); i++ {
			if freeSpaces[i].Size < bestSize {
				bestSize = freeSpaces[i].Size
				bestIndex = i
			}
		}
		return freeSpaces[bestIndex].Start, nil

	case 'W': // Worst Fit
		worstIndex := 0
		worstSize := freeSpaces[0].Size
		for i := 1; i < len(freeSpaces); i++ {
			if freeSpaces[i].Size > worstSize {
				worstSize = freeSpaces[i].Size
				worstIndex = i
			}
		}
		return freeSpaces[worstIndex].Start, nil

	default:
		// Por defecto usar First Fit
		return freeSpaces[0].Start, nil
	}
}

// Funciones auxiliares para crear cada tipo de partición
func createPrimaryPartition(mbr *structuresP2.MBR, file *os.File, name string, start, size int32, fit byte) {
	// Encontrar slot libre
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartS == 0 {
			mbr.MbrPartitions[i].PartStatus = '0'
			mbr.MbrPartitions[i].PartType = 'P'
			mbr.MbrPartitions[i].PartFit = fit
			mbr.MbrPartitions[i].PartStart = start
			mbr.MbrPartitions[i].PartS = size
			copy(mbr.MbrPartitions[i].PartName[:], name)

			// Escribir MBR actualizado
			if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
				fmt.Printf("Error escribiendo MBR: %s\n", err)
				return
			}

			fmt.Printf("\n=== PARTICIÓN PRIMARIA CREADA ===\n")

			return
		}
	}

	fmt.Println("Error: No hay slots disponibles para particiones")
}

func createExtendedPartition(mbr *structuresP2.MBR, file *os.File, name string, start, size int32, fit byte) {
	// Similar a createPrimaryPartition pero con Type = 'E'
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartS == 0 {
			mbr.MbrPartitions[i].PartStatus = '0'
			mbr.MbrPartitions[i].PartType = 'E'
			mbr.MbrPartitions[i].PartFit = fit
			mbr.MbrPartitions[i].PartStart = start
			mbr.MbrPartitions[i].PartS = size
			copy(mbr.MbrPartitions[i].PartName[:], name)

			// Escribir MBR actualizado
			if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
				fmt.Printf("Error escribiendo MBR: %s\n", err)
				return
			}

			fmt.Printf("\n=== PARTICIÓN EXTENDIDA CREADA ===\n")

			return
		}
	}
}

func createLogicalPartition(mbr *structuresP2.MBR, file *os.File, name string, start, size int32, fit byte) {
	// 1. Encontrar la partición extendida
	var extendedPartition *structuresP2.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		fmt.Println("Error: No se encontró partición extendida")
		return
	}

	// 2. Buscar espacio disponible dentro de la partición extendida
	ebrPosition, dataStart, err := findLogicalPartitionSpace(file, extendedPartition, size, fit, name)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// 3. Crear el nuevo EBR
	newEBR := structuresP2.NewEBRWithData(fit, dataStart, size, name, -1)

	// 4. Insertar el EBR en la cadena de EBRs
	if err := insertEBRInChain(file, extendedPartition, newEBR, ebrPosition); err != nil {
		fmt.Printf("Error insertando EBR en la cadena: %s\n", err)
		return
	}

	// 5. Escribir el nuevo EBR al disco
	if err := structuresP2.WriteEBR(file, newEBR, ebrPosition); err != nil {
		fmt.Printf("Error escribiendo EBR: %s\n", err)
		return
	}

	fmt.Printf("\n=== PARTICIÓN LÓGICA CREADA ===\n")

}

// findLogicalPartitionSpace busca espacio disponible dentro de la partición extendida
func findLogicalPartitionSpace(file *os.File, extendedPartition *structuresP2.Partition, size int32, fit byte, name string) (int32, int32, error) {
	ebrSize := int32(unsafe.Sizeof(structuresP2.EBR{}))
	requiredSpace := size + ebrSize // EBR + datos

	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	// Lista para almacenar todas las particiones lógicas existentes
	var existingLogicals []LogicalSpace // Ya no necesitas definir el tipo aquí

	// Recorrer la cadena de EBRs para encontrar particiones lógicas existentes
	currentEBRPos := extendedStart
	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil {
			// Si no se puede leer, no hay más EBRs válidos
			break
		}

		// Verificar si el EBR contiene una partición válida
		if !ebr.IsEmpty() {
			// Verificar nombre duplicado
			if ebr.GetName() == name {
				return 0, 0, fmt.Errorf("ya existe una partición lógica con el nombre '%s'", name)
			}

			existingLogicals = append(existingLogicals, LogicalSpace{
				EBRPos:    currentEBRPos,
				DataStart: ebr.PartStart,
				DataEnd:   ebr.PartStart + ebr.PartS,
				Name:      ebr.GetName(),
			})
		}

		// Avanzar al siguiente EBR
		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	// Buscar espacio libre según el algoritmo de ajuste
	switch fit {
	case 'F': // First Fit
		return findFirstFitLogical(extendedStart, extendedEnd, existingLogicals, requiredSpace, ebrSize)
	case 'B': // Best Fit
		return findBestFitLogical(extendedStart, extendedEnd, existingLogicals, requiredSpace, ebrSize)
	case 'W': // Worst Fit
		return findWorstFitLogical(extendedStart, extendedEnd, existingLogicals, requiredSpace, ebrSize)
	default:
		return findFirstFitLogical(extendedStart, extendedEnd, existingLogicals, requiredSpace, ebrSize)
	}

}

// insertEBRInChain inserta el EBR en la cadena de EBRs existente
func insertEBRInChain(file *os.File, extendedPartition *structuresP2.Partition, newEBR *structuresP2.EBR, newEBRPos int32) error {
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	// Verificar si es el primer EBR
	firstEBR, err := structuresP2.ReadEBR(file, extendedStart)
	if err != nil || firstEBR.IsEmpty() {
		// Es el primer EBR, no hay cadena que actualizar
		return nil
	}

	// Buscar la posición correcta en la cadena ordenada
	currentEBRPos := extendedStart
	var previousEBR *structuresP2.EBR
	var previousEBRPos int32

	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		currentEBR, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil || currentEBR.IsEmpty() {
			break
		}

		// Si la nueva partición debe ir antes de la actual
		if newEBRPos < currentEBRPos {
			newEBR.PartNext = currentEBRPos
			if previousEBR != nil {
				previousEBR.PartNext = newEBRPos
				// Escribir el EBR anterior actualizado
				if err := structuresP2.WriteEBR(file, previousEBR, previousEBRPos); err != nil {
					return err
				}
			}
			return nil
		}

		// Guardar referencia al EBR actual como el anterior
		previousEBR = currentEBR
		previousEBRPos = currentEBRPos

		// Avanzar al siguiente
		if !currentEBR.HasNext() {
			break
		}
		currentEBRPos = currentEBR.PartNext
	}

	// Si llegamos aquí, la nueva partición va al final
	if previousEBR != nil {
		previousEBR.PartNext = newEBRPos
		// Escribir el EBR anterior actualizado
		if err := structuresP2.WriteEBR(file, previousEBR, previousEBRPos); err != nil {
			return err
		}
	}

	return nil
}

// Algoritmos de ajuste para particiones lógicas
func findFirstFitLogical(extStart, extEnd int32, existing []LogicalSpace, requiredSpace, ebrSize int32) (int32, int32, error) {
	currentPos := extStart

	// Ordenar las particiones existentes por posición
	for i := 0; i < len(existing); i++ {
		for j := i + 1; j < len(existing); j++ {
			if existing[i].EBRPos > existing[j].EBRPos {
				existing[i], existing[j] = existing[j], existing[i]
			}
		}
	}

	// Buscar en los huecos entre particiones
	for _, logical := range existing {
		gapSize := logical.EBRPos - currentPos
		if gapSize >= requiredSpace {
			// Encontrado espacio suficiente
			ebrPos := currentPos
			dataStart := ebrPos + ebrSize
			return ebrPos, dataStart, nil
		}
		// Actualizar posición actual al final de esta partición lógica
		if logical.DataEnd > currentPos {
			currentPos = logical.DataEnd
		}
	}

	// Verificar espacio al final
	if extEnd-currentPos >= requiredSpace {
		ebrPos := currentPos
		dataStart := ebrPos + ebrSize
		return ebrPos, dataStart, nil
	}

	return 0, 0, fmt.Errorf("no hay espacio suficiente en la partición extendida para %d bytes", requiredSpace)
}

func findBestFitLogical(extStart, extEnd int32, existing []LogicalSpace, requiredSpace, ebrSize int32) (int32, int32, error) {
	bestEBRPos := int32(-1)
	bestDataStart := int32(-1)
	bestSize := int32(0x7FFFFFFF) // Máximo int32

	currentPos := extStart

	// Ordenar las particiones existentes por posición
	for i := 0; i < len(existing); i++ {
		for j := i + 1; j < len(existing); j++ {
			if existing[i].EBRPos > existing[j].EBRPos {
				existing[i], existing[j] = existing[j], existing[i]
			}
		}
	}

	// Buscar en los huecos entre particiones
	for _, logical := range existing {
		gapSize := logical.EBRPos - currentPos
		if gapSize >= requiredSpace && gapSize < bestSize {
			bestSize = gapSize
			bestEBRPos = currentPos
			bestDataStart = currentPos + ebrSize
		}
		if logical.DataEnd > currentPos {
			currentPos = logical.DataEnd
		}
	}

	// Verificar espacio al final
	gapSize := extEnd - currentPos
	if gapSize >= requiredSpace && gapSize < bestSize {
		bestEBRPos = currentPos
		bestDataStart = currentPos + ebrSize
	}

	if bestEBRPos == -1 {
		return 0, 0, fmt.Errorf("no hay espacio suficiente en la partición extendida para %d bytes", requiredSpace)
	}

	return bestEBRPos, bestDataStart, nil
}

func findWorstFitLogical(extStart, extEnd int32, existing []LogicalSpace, requiredSpace, ebrSize int32) (int32, int32, error) {
	worstEBRPos := int32(-1)
	worstDataStart := int32(-1)
	worstSize := int32(0)

	currentPos := extStart

	// Ordenar las particiones existentes por posición
	for i := 0; i < len(existing); i++ {
		for j := i + 1; j < len(existing); j++ {
			if existing[i].EBRPos > existing[j].EBRPos {
				existing[i], existing[j] = existing[j], existing[i]
			}
		}
	}

	// Buscar el hueco más grande
	for _, logical := range existing {
		gapSize := logical.EBRPos - currentPos
		if gapSize >= requiredSpace && gapSize > worstSize {
			worstSize = gapSize
			worstEBRPos = currentPos
			worstDataStart = currentPos + ebrSize
		}
		if logical.DataEnd > currentPos {
			currentPos = logical.DataEnd
		}
	}

	// Verificar espacio al final
	gapSize := extEnd - currentPos
	if gapSize >= requiredSpace && gapSize > worstSize {
		worstEBRPos = currentPos
		worstDataStart = currentPos + ebrSize
	}

	if worstEBRPos == -1 {
		return 0, 0, fmt.Errorf("no hay espacio suficiente en la partición extendida para %d bytes", requiredSpace)
	}

	return worstEBRPos, worstDataStart, nil
}

func deletePartition(mbr *structuresP2.MBR, file *os.File, name, deleteType string) {
	// Buscar la partición por nombre
	partitionFound := false
	var partitionIndex int = -1
	var partitionType byte

	// Buscar en particiones primarias y extendidas
	for i, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 {
			partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
			if partName == name {
				partitionFound = true
				partitionIndex = i
				partitionType = partition.PartType
				break
			}
		}
	}

	// Si no se encontró en particiones primarias/extendidas, buscar en lógicas
	if !partitionFound {
		if err := deleteLogicalPartition(mbr, file, name, deleteType); err != nil {
			fmt.Printf("Error: %s\n", err)
		}
		return
	}

	// Confirmar eliminación
	fmt.Printf("¿Está seguro de eliminar la partición '%s' (tipo: %c)? [s/N]: ", name, partitionType)
	// En un sistema real, aquí se leería la entrada del usuario
	// Por ahora, asumimos confirmación automática

	// Realizar eliminación según el tipo
	switch strings.ToUpper(deleteType) {
	case "FAST":
		deletePartitionFast(mbr, file, partitionIndex, name, partitionType)
	case "FULL":
		deletePartitionFull(mbr, file, partitionIndex, name, partitionType)
	default:
		fmt.Printf("Error: Tipo de eliminación '%s' no válido\n", deleteType)
	}
}

// deletePartitionFast elimina la partición solo limpiando los metadatos
func deletePartitionFast(mbr *structuresP2.MBR, file *os.File, index int, name string, partType byte) {
	// Limpiar la entrada en el MBR
	mbr.MbrPartitions[index] = structuresP2.Partition{}

	// Escribir MBR actualizado
	if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
		fmt.Printf("Error escribiendo MBR actualizado: %s\n", err)
		return
	}

	fmt.Printf("Partición '%s' (tipo: %c) eliminada exitosamente (FAST)\n", name, partType)
	fmt.Println("  Los datos permanecen en el disco pero la partición es inaccesible")
}

// deletePartitionFull elimina la partición y limpia completamente los datos
func deletePartitionFull(mbr *structuresP2.MBR, file *os.File, index int, name string, partType byte) {
	partition := mbr.MbrPartitions[index]

	// Llenar con ceros toda la partición
	fmt.Printf("Realizando eliminación completa de la partición '%s'...\n", name)

	// Crear buffer de ceros
	bufferSize := int32(4096) // 4KB buffer
	zeroBuffer := make([]byte, bufferSize)

	// Calcular cuántos bloques necesitamos escribir
	totalSize := partition.PartS
	blocksToWrite := (totalSize + bufferSize - 1) / bufferSize

	// Sobrescribir toda la partición con ceros
	currentPos := partition.PartStart
	for i := int32(0); i < blocksToWrite; i++ {
		writeSize := bufferSize
		if i == blocksToWrite-1 {
			// Último bloque, puede ser menor
			writeSize = totalSize - (i * bufferSize)
		}

		if _, err := file.Seek(int64(currentPos), 0); err != nil {
			fmt.Printf("Error posicionando en el archivo: %s\n", err)
			return
		}

		if _, err := file.Write(zeroBuffer[:writeSize]); err != nil {
			fmt.Printf("Error escribiendo ceros: %s\n", err)
			return
		}

		currentPos += writeSize

		// Mostrar progreso
		progress := float64(i+1) / float64(blocksToWrite) * 100
		fmt.Printf("  Progreso: %.1f%%\r", progress)
	}

	// Limpiar la entrada en el MBR
	mbr.MbrPartitions[index] = structuresP2.Partition{}

	// Escribir MBR actualizado
	if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
		fmt.Printf("Error escribiendo MBR actualizado: %s\n", err)
		return
	}

	fmt.Printf("\nPartición '%s' (tipo: %c) eliminada exitosamente (FULL)\n", name, partType)
	fmt.Printf("  Datos completamente borrados (%d bytes limpiados)\n", totalSize)
}

// deleteLogicalPartition elimina una partición lógica
func deleteLogicalPartition(mbr *structuresP2.MBR, file *os.File, name, deleteType string) error {
	// Encontrar la partición extendida
	var extendedPartition *structuresP2.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return fmt.Errorf("partición '%s' no encontrada", name)
	}

	// Buscar la partición lógica en la cadena de EBRs
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	currentEBRPos := extendedStart
	var previousEBRPos int32 = -1
	var previousEBR *structuresP2.EBR

	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		currentEBR, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !currentEBR.IsEmpty() && currentEBR.GetName() == name {
			// Encontramos la partición a eliminar
			fmt.Printf("Eliminando partición lógica '%s'...\n", name)

			if strings.ToUpper(deleteType) == "FULL" {
				// Eliminación completa: llenar con ceros
				zeroBuffer := make([]byte, 4096)
				totalSize := currentEBR.PartS
				blocksToWrite := (totalSize + 4096 - 1) / 4096

				writePos := currentEBR.PartStart
				for i := int32(0); i < blocksToWrite; i++ {
					writeSize := int32(4096)
					if i == blocksToWrite-1 {
						writeSize = totalSize - (i * 4096)
					}

					file.Seek(int64(writePos), 0)
					file.Write(zeroBuffer[:writeSize])
					writePos += writeSize
				}
			}

			// Actualizar la cadena de EBRs
			if previousEBR != nil {
				// No es el primer EBR, actualizar el anterior
				previousEBR.PartNext = currentEBR.PartNext
				if err := structuresP2.WriteEBR(file, previousEBR, previousEBRPos); err != nil {
					return fmt.Errorf("error actualizando EBR anterior: %s", err)
				}
			} else {
				// Es el primer EBR, crear un EBR vacío o mover el siguiente
				if currentEBR.PartNext != -1 {
					// Mover el siguiente EBR al inicio
					nextEBR, err := structuresP2.ReadEBR(file, currentEBR.PartNext)
					if err != nil {
						return fmt.Errorf("error leyendo siguiente EBR: %s", err)
					}
					if err := structuresP2.WriteEBR(file, nextEBR, extendedStart); err != nil {
						return fmt.Errorf("error escribiendo EBR movido: %s", err)
					}
				}
			}

			// Limpiar el EBR actual
			emptyEBR := structuresP2.NewEBR()
			if err := structuresP2.WriteEBR(file, emptyEBR, currentEBRPos); err != nil {
				return fmt.Errorf("error limpiando EBR: %s", err)
			}

			fmt.Printf("Partición lógica '%s' eliminada exitosamente (%s)\n", name, deleteType)
			return nil
		}

		// Guardar referencia al EBR actual y avanzar
		previousEBR = currentEBR
		previousEBRPos = currentEBRPos

		if !currentEBR.HasNext() {
			break
		}
		currentEBRPos = currentEBR.PartNext
	}

	return fmt.Errorf("partición lógica '%s' no encontrada", name)
}
func modifyPartition(mbr *structuresP2.MBR, file *os.File, name string, add int32) {
	// Buscar la partición por nombre
	partitionFound := false
	var partitionIndex int = -1

	// Buscar en particiones primarias y extendidas
	for i, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 {
			partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
			if partName == name {
				partitionFound = true
				partitionIndex = i
				break
			}
		}
	}

	// Si no se encontró en particiones primarias/extendidas, buscar en lógicas
	if !partitionFound {
		if err := modifyLogicalPartition(mbr, file, name, add); err != nil {
			fmt.Printf("Error: %s\n", err)
		}
		return
	}

	if add > 0 {
		// Agregar espacio
		if err := expandPartition(mbr, file, partitionIndex, add); err != nil {
			fmt.Printf("Error expandiendo partición: %s\n", err)
			return
		}
	} else if add < 0 {
		// Reducir espacio
		if err := shrinkPartition(mbr, file, partitionIndex, -add); err != nil {
			fmt.Printf("Error reduciendo partición: %s\n", err)
			return
		}
	} else {
		fmt.Println("Error: El valor de 'add' no puede ser cero")
	}
}

// expandPartition expande una partición agregando espacio
func expandPartition(mbr *structuresP2.MBR, file *os.File, index int, addSize int32) error {
	partition := &mbr.MbrPartitions[index]
	partName := strings.TrimRight(string(partition.PartName[:]), "\x00")

	// Verificar si hay espacio disponible después de la partición
	partitionEnd := partition.PartStart + partition.PartS
	diskSize := mbr.MbrTamano

	// Buscar la siguiente partición para verificar espacio disponible
	nextPartitionStart := diskSize // Asumir que no hay siguiente partición
	for _, p := range mbr.MbrPartitions {
		if p.PartS > 0 && p.PartStart > partitionEnd && p.PartStart < nextPartitionStart {
			nextPartitionStart = p.PartStart
		}
	}

	availableSpace := nextPartitionStart - partitionEnd
	if availableSpace < addSize {
		return fmt.Errorf("no hay suficiente espacio disponible. Requerido: %d bytes, Disponible: %d bytes",
			addSize, availableSpace)
	}

	// Expandir la partición
	oldSize := partition.PartS
	partition.PartS += addSize

	// Escribir MBR actualizado
	if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
		return fmt.Errorf("error escribiendo MBR actualizado: %s", err)
	}

	fmt.Printf("Partición '%s' expandida exitosamente\n", partName)
	fmt.Printf("  Tamaño anterior: %d bytes\n", oldSize)
	fmt.Printf("  Tamaño nuevo: %d bytes\n", partition.PartS)
	fmt.Printf("  Espacio agregado: %d bytes\n", addSize)

	return nil
}

// shrinkPartition reduce el tamaño de una partición
func shrinkPartition(mbr *structuresP2.MBR, file *os.File, index int, reduceSize int32) error {
	partition := &mbr.MbrPartitions[index]
	partName := strings.TrimRight(string(partition.PartName[:]), "\x00")

	if partition.PartS <= reduceSize {
		return fmt.Errorf("no se puede reducir la partición a un tamaño menor o igual a cero")
	}

	// Confirmar reducción (en un sistema real se pediría confirmación)
	fmt.Printf("ADVERTENCIA: Reducir la partición '%s' puede causar pérdida de datos\n", partName)

	oldSize := partition.PartS
	newSize := partition.PartS - reduceSize

	// Limpiar el espacio que se va a liberar (opcional, pero recomendado)
	zeroBuffer := make([]byte, 4096)
	blocksToClean := (reduceSize + 4096 - 1) / 4096

	cleanStartPos := partition.PartStart + newSize
	for i := int32(0); i < blocksToClean; i++ {
		writeSize := int32(4096)
		if i == blocksToClean-1 {
			writeSize = reduceSize - (i * 4096)
		}

		if _, err := file.Seek(int64(cleanStartPos+(i*4096)), 0); err != nil {
			return fmt.Errorf("error posicionando para limpiar: %s", err)
		}

		if _, err := file.Write(zeroBuffer[:writeSize]); err != nil {
			return fmt.Errorf("error limpiando espacio liberado: %s", err)
		}
	}

	// Actualizar el tamaño de la partición
	partition.PartS = newSize

	// Escribir MBR actualizado
	if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
		return fmt.Errorf("error escribiendo MBR actualizado: %s", err)
	}

	fmt.Printf("Partición '%s' reducida exitosamente\n", partName)
	fmt.Printf("  Tamaño anterior: %d bytes\n", oldSize)
	fmt.Printf("  Tamaño nuevo: %d bytes\n", partition.PartS)
	fmt.Printf("  Espacio liberado: %d bytes\n", reduceSize)

	return nil
}

// modifyLogicalPartition modifica una partición lógica
func modifyLogicalPartition(mbr *structuresP2.MBR, file *os.File, name string, add int32) error {
	// Encontrar la partición extendida
	var extendedPartition *structuresP2.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return fmt.Errorf("partición '%s' no encontrada", name)
	}

	// Buscar la partición lógica en la cadena de EBRs
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	currentEBRPos := extendedStart
	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		currentEBR, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !currentEBR.IsEmpty() && currentEBR.GetName() == name {
			// Encontramos la partición a modificar
			if add > 0 {
				// Expandir partición lógica
				return expandLogicalPartition(file, currentEBR, currentEBRPos, add, extendedEnd)
			} else if add < 0 {
				// Reducir partición lógica
				return shrinkLogicalPartition(file, currentEBR, currentEBRPos, -add)
			}
		}

		if !currentEBR.HasNext() {
			break
		}
		currentEBRPos = currentEBR.PartNext
	}

	return fmt.Errorf("partición lógica '%s' no encontrada", name)
}

// expandLogicalPartition expande una partición lógica
func expandLogicalPartition(file *os.File, ebr *structuresP2.EBR, ebrPos int32, addSize int32, extendedEnd int32) error {
	partitionEnd := ebr.PartStart + ebr.PartS

	// Verificar espacio disponible (hasta el final de la partición extendida o siguiente EBR)
	availableSpace := extendedEnd - partitionEnd
	if ebr.HasNext() {
		nextEBR, err := structuresP2.ReadEBR(file, ebr.PartNext)
		if err == nil {
			// Usar nextEBR correctamente:
			availableSpace = nextEBR.PartStart - partitionEnd // Cambio aquí
		}
	}

	if availableSpace < addSize {
		return fmt.Errorf("no hay suficiente espacio disponible. Requerido: %d bytes, Disponible: %d bytes",
			addSize, availableSpace)
	}

	oldSize := ebr.PartS
	ebr.PartS += addSize

	// Escribir EBR actualizado
	if err := structuresP2.WriteEBR(file, ebr, ebrPos); err != nil {
		return fmt.Errorf("error escribiendo EBR actualizado: %s", err)
	}

	fmt.Printf("Partición lógica '%s' expandida exitosamente\n", ebr.GetName())
	fmt.Printf("  Tamaño anterior: %d bytes\n", oldSize)
	fmt.Printf("  Tamaño nuevo: %d bytes\n", ebr.PartS)
	fmt.Printf("  Espacio agregado: %d bytes\n", addSize)

	return nil
}

// shrinkLogicalPartition reduce una partición lógica
func shrinkLogicalPartition(file *os.File, ebr *structuresP2.EBR, ebrPos int32, reduceSize int32) error {
	if ebr.PartS <= reduceSize {
		return fmt.Errorf("no se puede reducir la partición a un tamaño menor o igual a cero")
	}

	oldSize := ebr.PartS
	ebr.PartS -= reduceSize

	// Limpiar el espacio liberado
	zeroBuffer := make([]byte, 4096)
	blocksToClean := (reduceSize + 4096 - 1) / 4096

	cleanStartPos := ebr.PartStart + ebr.PartS
	for i := int32(0); i < blocksToClean; i++ {
		writeSize := int32(4096)
		if i == blocksToClean-1 {
			writeSize = reduceSize - (i * 4096)
		}

		file.Seek(int64(cleanStartPos+(i*4096)), 0)
		file.Write(zeroBuffer[:writeSize])
	}

	// Escribir EBR actualizado
	if err := structuresP2.WriteEBR(file, ebr, ebrPos); err != nil {
		return fmt.Errorf("error escribiendo EBR actualizado: %s", err)
	}

	fmt.Printf("Partición lógica '%s' reducida exitosamente\n", ebr.GetName())
	fmt.Printf("  Tamaño anterior: %d bytes\n", oldSize)
	fmt.Printf("  Tamaño nuevo: %d bytes\n", ebr.PartS)
	fmt.Printf("  Espacio liberado: %d bytes\n", reduceSize)

	return nil
}

// showDiskStatus muestra el estado completo del disco
func showDiskStatus(diskPath string) {
	fmt.Printf("\n--- ESTADO ACTUAL DEL DISCO ---\n")

	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		fmt.Printf("Error leyendo disco: %s\n", err)
		return
	}
	defer file.Close()

	var mbr structuresP2.MBR
	if err := utilitiesP2.ReadStruct(file, 0, &mbr); err != nil {
		fmt.Printf("Error leyendo MBR: %s\n", err)
		return
	}

	fmt.Printf("Disco: %s\n", diskPath)
	fmt.Printf("Signature: %d\n", mbr.MbrDskSignature)
	fmt.Printf("Tamaño total: %d bytes\n", mbr.MbrTamano)

	// Calcular espacio usado y libre
	totalUsed := int32(0)
	partitionsCount := 0

	fmt.Printf("\n PARTICIONES:\n")
	for i := 0; i < 4; i++ {
		partition := mbr.MbrPartitions[i]
		fmt.Printf("  [%d] ", i+1)

		if partition.PartS == 0 {
			fmt.Printf("Libre\n")
		} else {
			partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
			partType := map[byte]string{'P': "Primaria", 'E': "Extendida", 'L': "Lógica"}[partition.PartType]
			status := map[byte]string{'0': "No montada", '1': "Montada", '2': "Formateada"}[partition.PartStatus]

			fmt.Printf("%s\n", partName)
			fmt.Printf("      Tipo: %s\n", partType)
			fmt.Printf("      Tamaño: %d bytes\n", partition.PartS)
			fmt.Printf("      Inicio: %d\n", partition.PartStart)
			fmt.Printf("      Fin: %d\n", partition.PartStart+partition.PartS-1)
			fmt.Printf("      Fit: %c\n", partition.PartFit)
			fmt.Printf("      Estado: %s\n", status)

			if partition.PartStatus != '0' {
				partID := strings.TrimRight(string(partition.PartId[:]), "\x00")
				if partID != "" {
					fmt.Printf("      ID montaje: %s\n", partID)
				}
			}

			totalUsed += partition.PartS
			partitionsCount++

			// Si es extendida, mostrar particiones lógicas
			if partition.PartType == 'E' {
				showLogicalPartitions(file, partition)
			}
		}
	}

	freeSpace := mbr.MbrTamano - totalUsed
	fmt.Printf("\nRESUMEN:\n")
	fmt.Printf("   Particiones creadas: %d/4\n", partitionsCount)
	fmt.Printf("  Espacio usado: %d bytes (%.2f%%)\n", totalUsed, float64(totalUsed)/float64(mbr.MbrTamano)*100)
	fmt.Printf("  Espacio libre: %d bytes (%.2f%%)\n", freeSpace, float64(freeSpace)/float64(mbr.MbrTamano)*100)

	fmt.Printf("=====================================\n")
}

// showLogicalPartitions muestra las particiones lógicas dentro de una extendida
func showLogicalPartitions(file *os.File, extendedPartition structuresP2.Partition) {
	fmt.Printf("      PARTICIONES LÓGICAS:\n")

	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS
	currentEBRPos := extendedStart
	logicalCount := 0

	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !ebr.IsEmpty() {
			logicalCount++
			logicalName := ebr.GetName()
			status := map[byte]string{0: "No montada", 1: "Montada", 2: "Formateada"}[ebr.PartMount]

			fmt.Printf("        [L%d] %s\n", logicalCount, logicalName)
			fmt.Printf("             Tamaño: %d bytes\n", ebr.PartS)
			fmt.Printf("             Inicio: %d\n", ebr.PartStart)
			fmt.Printf("             Fin: %d\n", ebr.PartStart+ebr.PartS-1)
			fmt.Printf("             Fit: %c\n", ebr.PartFit)
			fmt.Printf("             Estado: %s\n", status)
			if ebr.HasNext() {
				fmt.Printf("             Siguiente EBR: %d\n", ebr.PartNext)
			}
		}

		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	if logicalCount == 0 {
		fmt.Printf("        Sin particiones lógicas\n")
	}
}

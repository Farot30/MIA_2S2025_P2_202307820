package commandsP2

import (
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	structuresP2 "MIA_2S2025_P2_202307820/structures"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// MountedPartition representa una partición montada en memoria
type MountedPartition struct {
	ID     string // ID único generado (ej: 341A, 342A, etc.)
	Path   string // Ruta del disco
	Name   string // Nombre de la partición
	DiskID string // Identificador único del disco para agrupación
	Start  int32  // Offset de inicio de la partición
	Type   string // Tipo de partición (Primaria, Lógica, Extendida)
}

var mountedPartitionsMutex sync.RWMutex

// MountManager maneja todas las particiones montadas
type MountManager struct {
	mutex             sync.RWMutex
	mountedPartitions map[string]MountedPartition // ID -> Partición montada
	diskCounters      map[string]int              // DiskID -> contador de particiones
	currentLetter     byte                        // Letra actual (A, B, C, etc.)
	carnetSuffix      string                      // Últimos 2 dígitos del carnet
}

// Instancia global del manager de montajes
var mountManager = &MountManager{
	mountedPartitions: make(map[string]MountedPartition),
	diskCounters:      make(map[string]int),
	currentLetter:     'A',
	carnetSuffix:      "20",
}

// MOUNT implementa el comando para montar particiones (primarias y lógicas)
func MOUNT(args []string) {
	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetros requeridos
	required := []string{"path", "name"}
	if err := utilitiesP2.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// Obtener parámetros
	path := params["path"]
	name := params["name"]

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

	// Buscar primero en particiones primarias
	partitionIndex := -1
	var partitionStart int32

	for i, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 {
			partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
			if partName == name {
				if partition.PartType == 'P' {
					// Partición primaria encontrada
					partitionIndex = i
					partitionStart = partition.PartStart
					break
				} else if partition.PartType == 'E' {
					fmt.Printf("Error: No se pueden montar particiones extendidas directamente. La partición '%s' es extendida\n", name)
					return
				}
			}
		}
	}

	// Si no se encontró en particiones primarias, buscar en particiones lógicas
	if partitionIndex == -1 {
		logicalStart, err := findAndMountLogicalPartition(file, &mbr, name, path)
		if err != nil {
			fmt.Printf("Error: No se encontró la partición '%s' en el disco '%s'\n", name, path)
			return
		}

		// Montar partición lógica
		mountLogicalPartition(path, name, logicalStart, &mbr, file)
		return
	}

	// Montar partición primaria
	mountPrimaryPartition(path, name, partitionIndex, partitionStart, &mbr, file)
}

// findAndMountLogicalPartition busca una partición lógica por nombre
func findAndMountLogicalPartition(file *os.File, mbr *structuresP2.MBR, name, diskPath string) (int32, error) {
	// Encontrar la partición extendida
	var extendedPartition *structuresP2.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return 0, fmt.Errorf("no hay partición extendida")
	}

	// Recorrer la cadena de EBRs
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	currentEBRPos := extendedStart
	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !ebr.IsEmpty() && ebr.GetName() == name {
			// Encontrada la partición lógica
			return ebr.PartStart, nil
		}

		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	return 0, fmt.Errorf("partición lógica no encontrada")
}

// mountPrimaryPartition monta una partición primaria
func mountPrimaryPartition(diskPath, partitionName string, partitionIndex int, partitionStart int32, mbr *structuresP2.MBR, file *os.File) {
	mountManager.mutex.Lock()
	defer mountManager.mutex.Unlock()

	// Crear un ID único para el disco basado en su ruta
	diskID := diskPath

	// Verificar si la partición ya está montada
	for id, mounted := range mountManager.mountedPartitions {
		if mounted.Path == diskPath && mounted.Name == partitionName {
			fmt.Printf("Error: La partición '%s' del disco '%s' ya está montada con ID '%s'\n",
				partitionName, diskPath, id)
			return
		}
	}

	// Obtener el número de partición para este disco
	partitionNumber := mountManager.diskCounters[diskID] + 1

	// Si es el primer montaje de un nuevo disco, verificar si necesitamos nueva letra
	if partitionNumber == 1 && len(mountManager.diskCounters) > 0 {
		// Solo incrementar letra si es un disco completamente diferente
		isDifferentDisk := true
		for existingDiskID := range mountManager.diskCounters {
			if existingDiskID == diskID {
				isDifferentDisk = false
				break
			}
		}

		if isDifferentDisk {
			mountManager.currentLetter++
		}
	}

	// Generar el ID: últimos 2 dígitos del carnet + número de partición + letra
	mountID := fmt.Sprintf("%s%d%c", mountManager.carnetSuffix, partitionNumber, mountManager.currentLetter)

	// Actualizar el contador para este disco
	mountManager.diskCounters[diskID] = partitionNumber

	// Actualizar el status de la partición en el MBR (marcar como montada)
	mbr.MbrPartitions[partitionIndex].PartStatus = 1 // 1 = montada, 0 = no montada
	mbr.MbrPartitions[partitionIndex].PartCorrelative = int32(partitionNumber)
	copy(mbr.MbrPartitions[partitionIndex].PartId[:], []byte(mountID))

	// Escribir el MBR actualizado al disco
	if err := utilitiesP2.WriteStruct(file, 0, mbr); err != nil {
		fmt.Printf("Error actualizando el MBR: %s\n", err)
		return
	}

	// Crear la entrada de partición montada con tipo "Primaria"
	mountedPartition := MountedPartition{
		ID:     mountID,
		Path:   diskPath,
		Name:   partitionName,
		DiskID: diskID,
		Start:  partitionStart,
		Type:   "Primaria", // Especificar que es primaria
	}

	// Agregar a la lista de particiones montadas
	mountManager.mountedPartitions[mountID] = mountedPartition

	fsType, err := detectFileSystemType(mountedPartition)
	if err == nil && fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		_, err := EstructuraExt3P2.ReadEXT3FileSystem(diskPath, partitionStart)
		if err == nil {
			LogEXT3Operation(EstructuraExt3P2.OPERATION_MOUNT, "/", fmt.Sprintf("Partición montada: %s", partitionName))
		}
	}

	// INFORMACIÓN DETALLADA DEL MONTAJE
	fmt.Printf("\n=== PARTICIÓN PRIMARIA MONTADA ===\n")
	fmt.Printf("ID generado: %s\n", mountID)
	fmt.Printf("Disco: %s\n", diskPath)
	fmt.Printf("Partición: %s\n", partitionName)
	fmt.Printf("Tipo: Primaria\n")
	fmt.Printf("Inicio: byte %d\n", partitionStart)
	fmt.Printf("Número en disco: %d\n", partitionNumber)
	fmt.Printf("Letra disco: %c\n", mountManager.currentLetter)

	// MOSTRAR INFORMACIÓN DETALLADA DE LA PARTICIÓN
	showMountedPartitionDetails(file, mbr, partitionIndex, diskPath)

	// MOSTRAR ESTADO ACTUAL DE TODAS LAS PARTICIONES MONTADAS
	showCurrentMountStatus()
}

// mountLogicalPartition monta una partición lógica
func mountLogicalPartition(diskPath, partitionName string, partitionStart int32, mbr *structuresP2.MBR, file *os.File) {
	mountManager.mutex.Lock()
	defer mountManager.mutex.Unlock()

	// Crear un ID único para el disco basado en su ruta
	diskID := diskPath

	// Verificar si la partición ya está montada
	for id, mounted := range mountManager.mountedPartitions {
		if mounted.Path == diskPath && mounted.Name == partitionName {
			fmt.Printf("Error: La partición '%s' del disco '%s' ya está montada con ID '%s'\n",
				partitionName, diskPath, id)
			return
		}
	}

	// Obtener el número de partición para este disco
	partitionNumber := mountManager.diskCounters[diskID] + 1

	// Si es el primer montaje de un nuevo disco, verificar si necesitamos nueva letra
	if partitionNumber == 1 && len(mountManager.diskCounters) > 0 {
		// Solo incrementar letra si es un disco completamente diferente
		isDifferentDisk := true
		for existingDiskID := range mountManager.diskCounters {
			if existingDiskID == diskID {
				isDifferentDisk = false
				break
			}
		}

		if isDifferentDisk {
			mountManager.currentLetter++
		}
	}

	// Generar el ID: últimos 2 dígitos del carnet + número de partición + letra
	mountID := fmt.Sprintf("%s%d%c", mountManager.carnetSuffix, partitionNumber, mountManager.currentLetter)

	// Actualizar el contador para este disco
	mountManager.diskCounters[diskID] = partitionNumber

	// Actualizar el EBR para marcar como montada
	if err := updateLogicalPartitionStatus(file, mbr, partitionName, partitionNumber, mountID); err != nil {
		fmt.Printf("Error actualizando EBR: %s\n", err)
		return
	}

	// Crear la entrada de partición montada con tipo "Lógica"
	mountedPartition := MountedPartition{
		ID:     mountID,
		Path:   diskPath,
		Name:   partitionName,
		DiskID: diskID,
		Start:  partitionStart,
		Type:   "Lógica", // Especificar que es lógica
	}

	// Agregar a la lista de particiones montadas
	mountManager.mountedPartitions[mountID] = mountedPartition

	fsType, err := detectFileSystemType(mountedPartition)
	if err == nil && fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		_, err := EstructuraExt3P2.ReadEXT3FileSystem(diskPath, partitionStart)
		if err == nil {
			LogEXT3Operation(EstructuraExt3P2.OPERATION_MOUNT, "/", fmt.Sprintf("Partición lógica montada: %s", partitionName))
		}
	}

	fmt.Printf("\n=== PARTICIÓN LÓGICA MONTADA ===\n")
	fmt.Printf("ID generado: %s\n", mountID)
	fmt.Printf("Disco: %s\n", diskPath)
	fmt.Printf(" Partición: %s\n", partitionName)
	fmt.Printf("Tipo: Lógica\n")
	fmt.Printf("Inicio: byte %d\n", partitionStart)
	fmt.Printf("Número en disco: %d\n", partitionNumber)
	fmt.Printf("Letra disco: %c\n", mountManager.currentLetter)

	// MOSTRAR ESTADO ACTUAL DE TODAS LAS PARTICIONES MONTADAS
	showCurrentMountStatus()
}

// updateLogicalPartitionStatus actualiza el status de una partición lógica
func updateLogicalPartitionStatus(file *os.File, mbr *structuresP2.MBR, partitionName string, partitionNumber int, mountID string) error {
	// Encontrar la partición extendida
	var extendedPartition *structuresP2.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return fmt.Errorf("no hay partición extendida")
	}

	// Buscar la partición lógica en la cadena de EBRs
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	currentEBRPos := extendedStart
	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !ebr.IsEmpty() && ebr.GetName() == partitionName {
			// Encontrada la partición lógica, actualizar status
			ebr.PartMount = 1 // 1 = montada, 0 = no montada

			// Escribir EBR actualizado
			if err := structuresP2.WriteEBR(file, ebr, currentEBRPos); err != nil {
				return fmt.Errorf("error escribiendo EBR actualizado: %s", err)
			}

			return nil
		}

		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	return fmt.Errorf("partición lógica no encontrada para actualizar")
}

// GetMountedPartition obtiene información de una partición montada por ID
func GetMountedPartition(id string) (MountedPartition, bool) {
	mountManager.mutex.RLock()
	defer mountManager.mutex.RUnlock()

	partition, exists := mountManager.mountedPartitions[strings.ToUpper(id)]
	return partition, exists
}

func showMountedPartitionDetails(file *os.File, mbr *structuresP2.MBR, partitionIndex int, diskPath string) {
	partition := mbr.MbrPartitions[partitionIndex]

	fmt.Printf("\n--- DETALLES DE LA PARTICIÓN ---\n")
	fmt.Printf("Tamaño: %d bytes\n", partition.PartS)
	fmt.Printf("Rango: bytes %d - %d\n", partition.PartStart, partition.PartStart+partition.PartS-1)
	fmt.Printf("Fit: %c\n", partition.PartFit)
	fmt.Printf("Status: Montada (%c)\n", partition.PartStatus)
	fmt.Printf("Correlativo: %d\n", partition.PartCorrelative)

	partID := strings.TrimRight(string(partition.PartId[:]), "\x00")
	fmt.Printf("ID en MBR: %s\n", partID)

	// Verificar si tiene sistema de archivos
	checkFileSystem(file, partition.PartStart)
}

// checkFileSystem verifica si la partición tiene sistema de archivos
func checkFileSystem(file *os.File, partitionStart int32) {
	fmt.Printf("\n--- SISTEMA DE ARCHIVOS ---\n")

	// Intentar leer superblock EXT2
	if _, err := file.Seek(int64(partitionStart), 0); err != nil {
		fmt.Printf("Error accediendo a la partición\n")
		return
	}

	// Leer primeros bytes para detectar formato
	buffer := make([]byte, 1024)
	if _, err := file.Read(buffer); err != nil {
		fmt.Printf("Error leyendo partición\n")
		return
	}

	// Buscar magic number de EXT2 o verificar si hay datos
	hasData := false
	for _, b := range buffer {
		if b != 0 {
			hasData = true
			break
		}
	}

	if hasData {
		fmt.Printf("Contiene datos/sistema de archivos\n")

		// Intentar detectar EXT2 específicamente
		ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(file.Name(), int32(partitionStart))
		if err == nil && ext2fs.SuperBlock.S_magic == estructuraArchivoCarpeta.EXT2_MAGIC {
			fmt.Printf("Tipo: EXT2\n")
			fmt.Printf("Inodos: %d (libres: %d)\n", ext2fs.SuperBlock.S_inodes_count, ext2fs.SuperBlock.S_free_inodes_count)
			fmt.Printf("Bloques: %d (libres: %d)\n", ext2fs.SuperBlock.S_blocks_count, ext2fs.SuperBlock.S_free_blocks_count)
		} else {
			fmt.Printf("Tipo: Desconocido/Raw\n")
		}
	} else {
		fmt.Printf("Partición sin formato (vacía)\n")
	}
}

// showCurrentMountStatus muestra el estado actual de montajes
func showCurrentMountStatus() {
	fmt.Printf("\n--- ESTADO ACTUAL DE MONTAJES ---\n")

	if len(mountManager.mountedPartitions) == 0 {
		fmt.Printf("No hay particiones montadas\n")
		return
	}

	fmt.Printf("Particiones montadas: %d\n", len(mountManager.mountedPartitions))

	// Agrupar por disco
	diskGroups := make(map[string][]MountedPartition)
	for _, partition := range mountManager.mountedPartitions {
		diskGroups[partition.DiskID] = append(diskGroups[partition.DiskID], partition)
	}

	fmt.Printf("=====================================\n")
}

func UNMOUNT(args []string) {
	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	required := []string{"id"}
	if err := utilitiesP2.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	id := strings.ToUpper(params["id"])

	MountedPartition, exists := GetMountedPartition(id)
	if !exists {
		fmt.Printf("Error: No hay ninguna partición montada con ID '%s'\n", id)
		return
	}

	mountManager.mutex.Lock()
	defer mountManager.mutex.Unlock()

	// Abrir el archivo del disco
	file, err := os.OpenFile(MountedPartition.Path, os.O_RDWR, 0666)
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

	if MountedPartition.Type == "Primaria" {
		// Buscar la partición primaria en el MBR y actualizar su estado
		for i := range mbr.MbrPartitions {
			partName := strings.TrimRight(string(mbr.MbrPartitions[i].PartName[:]), "\x00")
			if partName == MountedPartition.Name {
				mbr.MbrPartitions[i].PartStatus = 0 // Marcar como no montada
				mbr.MbrPartitions[i].PartCorrelative = 0
				mbr.MbrPartitions[i].PartId = [4]byte{}
				break
			}
		}
	} else if MountedPartition.Type == "Lógica" {
		// Actualizar el EBR para marcar la partición lógica como no montada
		if err := updateLogicalPartitionStatus(file, &mbr, MountedPartition.Name, 0, ""); err != nil {
			fmt.Printf("Error actualizando EBR: %s\n", err)
			return
		}
	}

	// Escribir el MBR actualizado al disco
	if err := utilitiesP2.WriteStruct(file, 0, &mbr); err != nil {
		fmt.Printf("Error actualizando el MBR: %s\n", err)
		return
	}

	fsType, err := detectFileSystemType(MountedPartition)
	if err == nil && fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		_, err := EstructuraExt3P2.ReadEXT3FileSystem(MountedPartition.Path, MountedPartition.Start)
		if err == nil {
			LogEXT3Operation(EstructuraExt3P2.OPERATION_UNMOUNT, "/", fmt.Sprintf("Partición desmontada: %s", MountedPartition.Name))
		}
	}

	// Eliminar de la lista de particiones montadas
	delete(mountManager.mountedPartitions, id)

	fmt.Printf("Partición con ID '%s' desmontada exitosamente\n", id)

	// Mostrar estado actual de montajes
	showCurrentMountStatus()
}

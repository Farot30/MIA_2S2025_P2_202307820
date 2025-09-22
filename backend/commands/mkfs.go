package commands

import (
	bloques "MIA_2S2025_P1_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	estructuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"MIA_2S2025_P1_202307820/structures"
	"MIA_2S2025_P1_202307820/utilities"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// UserManager maneja los IDs de usuarios y grupos
type UserManager struct {
	nextGroupID int32
	nextUserID  int32
}

// GetNextGroupID retorna el siguiente ID disponible para grupos
func (um *UserManager) GetNextGroupID() int32 {
	um.nextGroupID++
	return um.nextGroupID
}

// GetNextUserID retorna el siguiente ID disponible para usuarios
func (um *UserManager) GetNextUserID() int32 {
	um.nextUserID++
	return um.nextUserID
}

// InitializeUserSystem inicializa el sistema con valores por defecto
func InitializeUserSystem() string {
	um := &UserManager{
		nextGroupID: 0, // Empezar en 0, el primer grupo será 1
		nextUserID:  0, // Empezar en 0, el primer usuario será 1
	}

	// Crear grupo root (ID = 1)
	rootGroupID := um.GetNextGroupID()

	// Crear usuario root (ID = 1)
	rootUserID := um.GetNextUserID()

	return fmt.Sprintf("%d,G,root\n%d,U,root,root,123\n",
		rootGroupID, rootUserID)
}

// parseCurrentIDs analiza el contenido actual y encuentra los próximos IDs
func parseCurrentIDs(content string) *UserManager {
	lines := strings.Split(content, "\n")
	maxGroupID := int32(0)
	maxUserID := int32(0)

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			id, err := strconv.ParseInt(parts[0], 10, 32)
			if err == nil && id > 0 { // Solo contar IDs válidos (no eliminados)
				if parts[1] == "G" && int32(id) > maxGroupID {
					maxGroupID = int32(id)
				} else if parts[1] == "U" && int32(id) > maxUserID {
					maxUserID = int32(id)
				}
			}
		}
	}

	return &UserManager{
		nextGroupID: maxGroupID,
		nextUserID:  maxUserID,
	}
}

// AddUser agrega un nuevo usuario al sistema
func AddUser(currentContent, username, password, group string) string {
	um := parseCurrentIDs(currentContent) // Analizar IDs existentes
	newUserID := um.GetNextUserID()

	newEntry := fmt.Sprintf("%d,U,%s,%s,%s\n", newUserID, group, username, password)
	return currentContent + newEntry
}

// AddGroup agrega un nuevo grupo al sistema
func AddGroup(currentContent, groupname string) string {
	um := parseCurrentIDs(currentContent) // Analizar IDs existentes
	newGroupID := um.GetNextGroupID()

	newEntry := fmt.Sprintf("%d,G,%s\n", newGroupID, groupname)
	return currentContent + newEntry
}

// DeleteUser marca un usuario como eliminado (ID = 0)
func DeleteUser(currentContent, username string) string {
	lines := strings.Split(currentContent, "\n")
	var result []string

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 4 && parts[1] == "U" && parts[3] == username {
			// Marcar como eliminado
			parts[0] = "0"
			line = strings.Join(parts, ",")
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n") + "\n"
}

// DeleteGroup marca un grupo como eliminado (ID = 0)
func DeleteGroup(currentContent, groupname string) string {
	lines := strings.Split(currentContent, "\n")
	var result []string

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 3 && parts[1] == "G" && parts[2] == groupname {
			// Marcar como eliminado
			parts[0] = "0"
			line = strings.Join(parts, ",")
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n") + "\n"
}

// MKFS implementa el comando para formatear particiones con sistema EXT2
func MKFS(args []string) {
	// Parsear parámetros
	params := utilities.ParseParameters(args)

	// Validar parámetros requeridos
	required := []string{"id"}
	if err := utilities.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// Obtener parámetros
	id := strings.ToUpper(params["id"])
	formatType := strings.ToUpper(params["type"])

	// Validar tipo de formateo (por defecto Full)
	if formatType == "" {
		formatType = "FULL"
	}

	if formatType != "FULL" {
		fmt.Printf("Error: Tipo de formateo '%s' no válido. Use 'full' o deje vacío para formateo completo\n", formatType)
		return
	}

	// Buscar la partición montada por ID
	mountedPartition, exists := GetMountedPartition(id)
	if !exists {
		fmt.Printf("Error: No se encontró ninguna partición montada con ID '%s'\n", id)
		return
	}

	fmt.Printf("Iniciando formateo %s de la partición:\n", formatType)
	fmt.Printf("  ID: %s\n", mountedPartition.ID)
	fmt.Printf("  Disco: %s\n", mountedPartition.Path)
	fmt.Printf("  Partición: %s\n", mountedPartition.Name)
	fmt.Printf("  Tipo: %s\n", mountedPartition.Type)

	// Abrir el archivo del disco
	file, err := os.OpenFile(mountedPartition.Path, os.O_RDWR, 0666)
	if err != nil {
		fmt.Printf("Error abriendo el disco: %s\n", err)
		return
	}
	defer file.Close()

	// Obtener información de la partición según su tipo
	var partitionStart int32
	var partitionSize int32

	if mountedPartition.Type == "Primaria" {
		// Buscar en particiones primarias del MBR
		var mbr structures.MBR
		if err := utilities.ReadStruct(file, 0, &mbr); err != nil {
			fmt.Printf("Error leyendo el MBR: %s\n", err)
			return
		}

		var partition *structures.Partition
		var partitionIndex int = -1
		for i, p := range mbr.MbrPartitions {
			if p.PartS > 0 {
				partName := strings.TrimRight(string(p.PartName[:]), "\x00")
				if partName == mountedPartition.Name && p.PartType == 'P' {
					partition = &p
					partitionIndex = i
					partitionStart = p.PartStart
					partitionSize = p.PartS
					break
				}
			}
		}

		if partition == nil {
			fmt.Printf("Error: No se encontró la partición primaria '%s' en el MBR\n", mountedPartition.Name)
			return
		}

		// Formatear la partición primaria
		err = formatPartitionEXT2(file, partitionStart, partitionSize)
		if err != nil {
			fmt.Printf("Error formateando la partición: %s\n", err)
			return
		}

		// Actualizar el status de la partición en el MBR (marcar como formateada)
		mbr.MbrPartitions[partitionIndex].PartStatus = '2' // 2 = formateada
		if err := utilities.WriteStruct(file, 0, &mbr); err != nil {
			fmt.Printf("Error actualizando el MBR: %s\n", err)
			return
		}

	} else if mountedPartition.Type == "Lógica" {
		// Buscar en particiones lógicas (EBRs)
		partitionStart, partitionSize, err = findLogicalPartitionInfo(file, mountedPartition.Name)
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			return
		}

		// Formatear la partición lógica
		err = formatPartitionEXT2(file, partitionStart, partitionSize)
		if err != nil {
			fmt.Printf("Error formateando la partición: %s\n", err)
			return
		}

		// Para particiones lógicas, el status se actualiza en el EBR
		err = updateLogicalPartitionFormat(file, mountedPartition.Name)
		if err != nil {
			fmt.Printf("Error actualizando status de partición lógica: %s\n", err)
			return
		}
	} else {
		fmt.Printf("Error: Tipo de partición '%s' no soportado para formateo\n", mountedPartition.Type)
		return
	}

	fmt.Printf("Partición encontrada:\n")
	fmt.Printf("  Tipo: %s\n", mountedPartition.Type)
	fmt.Printf("  Inicio: %d\n", partitionStart)
	fmt.Printf("  Tamaño: %d bytes\n", partitionSize)

	// Crear el archivo users.txt en el sistema de archivos
	err = createUsersFile(file, partitionStart)
	if err != nil {
		fmt.Printf("Error creando el archivo users.txt: %s\n", err)
		return
	}

	fmt.Printf("\n=== FORMATEO COMPLETADO EXITOSAMENTE ===\n")
	fmt.Printf("Partición %s (%s) formateada como EXT2\n", mountedPartition.Name, id)
	fmt.Printf("Archivo users.txt creado en la raíz del sistema\n")
}

// findLogicalPartitionInfo busca información de una partición lógica
func findLogicalPartitionInfo(file *os.File, partitionName string) (int32, int32, error) {
	// Leer MBR para encontrar la partición extendida
	var mbr structures.MBR
	if err := utilities.ReadStruct(file, 0, &mbr); err != nil {
		return 0, 0, fmt.Errorf("error leyendo el MBR: %s", err)
	}

	// Encontrar la partición extendida
	var extendedPartition *structures.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return 0, 0, fmt.Errorf("no hay partición extendida")
	}

	// Buscar en la cadena de EBRs
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	currentEBRPos := extendedStart
	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structures.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !ebr.IsEmpty() && ebr.GetName() == partitionName {
			// Encontrada la partición lógica
			return ebr.PartStart, ebr.PartS, nil
		}

		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	return 0, 0, fmt.Errorf("partición lógica '%s' no encontrada", partitionName)
}

// updateLogicalPartitionFormat actualiza el status de formato de una partición lógica
func updateLogicalPartitionFormat(file *os.File, partitionName string) error {
	// Leer MBR para encontrar la partición extendida
	var mbr structures.MBR
	if err := utilities.ReadStruct(file, 0, &mbr); err != nil {
		return fmt.Errorf("error leyendo el MBR: %s", err)
	}

	// Encontrar la partición extendida
	var extendedPartition *structures.Partition
	for i := range mbr.MbrPartitions {
		if mbr.MbrPartitions[i].PartType == 'E' && mbr.MbrPartitions[i].PartS > 0 {
			extendedPartition = &mbr.MbrPartitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return fmt.Errorf("no hay partición extendida")
	}

	// Buscar en la cadena de EBRs
	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS

	currentEBRPos := extendedStart
	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structures.ReadEBR(file, currentEBRPos)
		if err != nil {
			break
		}

		if !ebr.IsEmpty() && ebr.GetName() == partitionName {
			// Encontrada la partición lógica - actualizar su status

			// Marcar como formateada en el campo PartMount
			// (Usamos PartMount como indicador: 0=no montada, 1=montada, 2=formateada)
			originalMount := ebr.PartMount
			ebr.PartMount = 2 // 2 = formateada

			// Escribir el EBR actualizado al disco
			err = structures.WriteEBR(file, ebr, currentEBRPos)
			if err != nil {
				return fmt.Errorf("error escribiendo EBR actualizado: %s", err)
			}

			fmt.Printf("✓ Status de partición lógica actualizado:\n")
			fmt.Printf("  Partición: %s\n", partitionName)
			fmt.Printf("  Status anterior: %d\n", originalMount)
			fmt.Printf("  Status actual: %d (formateada)\n", ebr.PartMount)
			fmt.Printf("  EBR posición: %d\n", currentEBRPos)

			return nil
		}

		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	return fmt.Errorf("partición lógica '%s' no encontrada para actualizar", partitionName)
}

// formatPartitionEXT2 formatea una partición con el sistema de archivos EXT2
func formatPartitionEXT2(file *os.File, partitionStart, partitionSize int32) error {
	fmt.Println("\nFormateando partición con sistema EXT2...")

	// Usar la función del módulo EXT2 para formatear
	err := EstructuraExt2.FormatPartitionForFDISK(
		file.Name(),
		partitionSize,
		partitionStart,
	)
	if err != nil {
		return fmt.Errorf("error en formateo EXT2: %s", err)
	}

	// Verificar que el formateo fue exitoso
	if !EstructuraExt2.IsEXT2Formatted(file.Name(), partitionStart) {
		return fmt.Errorf("el formateo EXT2 no se completó correctamente")
	}

	fmt.Println("✓ Sistema EXT2 creado exitosamente")
	return nil
}

// createUsersFile crea el archivo users.txt en la raíz del sistema de archivos
func createUsersFile(file *os.File, partitionStart int32) error {
	fmt.Println("\nCreando archivo users.txt...")

	// Leer el sistema EXT2 formateado
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(file.Name(), partitionStart)
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// Usar sistema de IDs incrementales en lugar de contenido fijo
	usersContent := InitializeUserSystem()

	// Crear el archivo users.txt usando las estructuras EXT2
	err = createFileInEXT2(file, ext2fs, "users.txt", usersContent)
	if err != nil {
		return fmt.Errorf("error creando users.txt: %s", err)
	}

	fmt.Println("✓ Archivo users.txt creado exitosamente")
	fmt.Println("  Contenido: Sistema de usuarios inicializado")
	fmt.Println("  Grupo root creado con ID 1")
	fmt.Println("  Usuario root creado con ID 1, contraseña '123'")
	fmt.Printf("  Estructura: %s", usersContent)

	return nil
}

func createFileInEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, filename, content string) error {
	// 1. Buscar un inodo libre
	inodeNum, err := findFreeInode(file, ext2fs)
	if err != nil {
		return err
	}

	// 2. Buscar un bloque libre
	blockNum, err := findFreeBlock(file, ext2fs)
	if err != nil {
		return err
	}

	// 3. Crear el inodo del archivo usando los métodos setter
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	fileInode.SetFileValues(
		1,                   // Cambio: UID 1 (usuario root según especificación)
		1,                   // Cambio: GID 1 (grupo root según especificación)
		int32(len(content)), // Tamaño del contenido
		1,                   // Tipo: archivo
		[3]byte{6, 4, 4},    // Permisos: rw-r--r-- (solo root puede modificar)
	)

	// Establecer el primer bloque
	fileInode.SetIblock(0, blockNum)

	// 4. Escribir el inodo
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (inodeNum * int32(ext2fs.SuperBlock.S_inode_s)))
	err = utilities.WriteStruct(file, inodeOffset, fileInode)
	if err != nil {
		return err
	}

	// 5. Crear el bloque de archivo con el contenido
	fileBlock := &bloques.BloqueArchivo{}
	copy(fileBlock.B_content[:], content)

	// 6. Escribir el bloque - CORREGIR CONVERSIÓN DE TIPOS
	blockOffset := int64(ext2fs.SuperBlock.S_block_start) + (int64(blockNum) * int64(ext2fs.SuperBlock.S_block_s))
	err = utilities.WriteStruct(file, blockOffset, fileBlock)
	if err != nil {
		return err
	}

	// 7. Actualizar los bitmaps
	err = updateBitmaps(file, ext2fs, inodeNum, blockNum)
	if err != nil {
		return err
	}

	// 8. Agregar la entrada al directorio raíz
	err = addFileToRootDirectory(file, ext2fs, filename, inodeNum)
	if err != nil {
		return err
	}

	// 9. Actualizar contadores en el superbloque
	ext2fs.SuperBlock.S_free_inodes_count--
	ext2fs.SuperBlock.S_free_blocks_count--

	// 10. Escribir el superbloque actualizado
	err = utilities.WriteStruct(file, int64(ext2fs.StartOffset), ext2fs.SuperBlock)
	if err != nil {
		return err
	}

	return nil
}

// findFreeInode encuentra el primer inodo libre
func findFreeInode(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem) (int32, error) {
	// Leer el bitmap de inodos
	bitmapSize := (ext2fs.SuperBlock.S_inodes_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return -1, err
	}

	_, err = file.Read(bitmap)
	if err != nil {
		return -1, err
	}

	// Buscar el primer bit en 0 (libre)
	for i := int32(1); i < ext2fs.SuperBlock.S_inodes_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		if (bitmap[byteIndex] & (1 << (7 - bitIndex))) == 0 {
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres disponibles")
}

// findFreeBlock encuentra el primer bloque libre
func findFreeBlock(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem) (int32, error) {
	// Leer el bitmap de bloques
	bitmapSize := (ext2fs.SuperBlock.S_blocks_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return -1, err
	}

	_, err = file.Read(bitmap)
	if err != nil {
		return -1, err
	}

	// Buscar el primer bit en 0 (libre)
	for i := int32(1); i < ext2fs.SuperBlock.S_blocks_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		if (bitmap[byteIndex] & (1 << (7 - bitIndex))) == 0 {
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay bloques libres disponibles")
}

// updateBitmaps actualiza los bitmaps de inodos y bloques
func updateBitmaps(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum, blockNum int32) error {
	// Actualizar bitmap de inodos
	bitmapInodeSize := (ext2fs.SuperBlock.S_inodes_count + 7) / 8
	inodeBitmap := make([]byte, bitmapInodeSize)

	// Leer bitmap actual
	_, err := file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return err
	}
	_, err = file.Read(inodeBitmap)
	if err != nil {
		return err
	}

	// Marcar inodo como usado
	byteIndex := inodeNum / 8
	bitIndex := inodeNum % 8
	inodeBitmap[byteIndex] |= (1 << (7 - bitIndex))

	// Escribir bitmap actualizado
	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return err
	}
	_, err = file.Write(inodeBitmap)
	if err != nil {
		return err
	}

	// Actualizar bitmap de bloques
	bitmapBlockSize := (ext2fs.SuperBlock.S_blocks_count + 7) / 8
	blockBitmap := make([]byte, bitmapBlockSize)

	// Leer bitmap actual
	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return err
	}
	_, err = file.Read(blockBitmap)
	if err != nil {
		return err
	}

	// Marcar bloque como usado
	byteIndex = blockNum / 8
	bitIndex = blockNum % 8
	blockBitmap[byteIndex] |= (1 << (7 - bitIndex))

	// Escribir bitmap actualizado
	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return err
	}
	_, err = file.Write(blockBitmap)
	return err
}

// addFileToRootDirectory agrega una entrada de archivo al directorio raíz
func addFileToRootDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, filename string, inodeNum int32) error {
	// Leer el bloque del directorio raíz
	rootBlock := &bloques.BloqueCarpeta{}

	rootBlockOffset := int64(ext2fs.SuperBlock.S_block_start)
	err := utilities.ReadStruct(file, rootBlockOffset, rootBlock)
	if err != nil {
		return err
	}

	// Buscar una entrada vacía en el directorio raíz
	for i := 0; i < 4; i++ {
		if rootBlock.IsEntryEmpty(i) {
			rootBlock.SetEntry(i, filename, inodeNum)
			break
		}

		// Si llegamos al final sin encontrar espacio
		if i == 3 {
			return fmt.Errorf("directorio raíz lleno, no se puede agregar '%s'", filename)
		}
	}

	// Escribir el bloque del directorio actualizado
	return utilities.WriteStruct(file, rootBlockOffset, rootBlock)
}

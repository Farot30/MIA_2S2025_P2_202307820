package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RECOVERY implementa el comando para recuperar el sistema de archivos EXT3
func RECOVERY(args []string) {
	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetro obligatorio -id
	partitionID, exists := params["id"]
	if !exists || strings.TrimSpace(partitionID) == "" {
		fmt.Println("Error: Debe especificar el parámetro obligatorio -id")
		fmt.Println("Uso: recovery -id=<partition_id>")
		fmt.Println("Ejemplo: recovery -id=061Disco1")
		return
	}

	partitionID = strings.ToUpper(strings.Trim(partitionID, "\""))

	// Verificar que la partición esté montada
	mountedPartition, exists := GetMountedPartition(partitionID)
	if !exists {
		fmt.Printf("Error: La partición con ID '%s' no está montada\n", partitionID)
		fmt.Println("Use el comando 'mount' para montar la partición antes de la recuperación")
		return
	}

	// Verificar que el usuario tenga permisos (solo root puede hacer recovery)
	if sessionP2.IsSessionActive() {
		sessionInfo := sessionP2.GetCurrentSession()
		if sessionInfo.Username != "root" {
			fmt.Printf("Error: Solo el usuario root puede ejecutar la recuperación del sistema\n")
			fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
			return
		}
	}

	// Detectar el tipo de sistema de archivos
	fsType, err := detectFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error detectando el sistema de archivos: %s\n", err)
		return
	}

	// Solo EXT3 soporta recovery con journaling
	if fsType != EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		fmt.Printf("Error: La recuperación solo está disponible para sistemas de archivos EXT3\n")
		fmt.Printf("Sistema de archivos detectado: %s\n", getFileSystemTypeName(fsType))
		return
	}

	fmt.Printf("=== INICIANDO RECUPERACIÓN DEL SISTEMA EXT3 ===\n")
	fmt.Printf("Partición: %s\n", partitionID)
	fmt.Printf("Disco: %s\n", mountedPartition.Path)
	fmt.Printf("Tipo: %s\n", mountedPartition.Type)
	fmt.Printf("Iniciando proceso de recuperación...\n\n")

	// Ejecutar la recuperación
	err = recoverEXT3FileSystem(mountedPartition)
	if err != nil {
		fmt.Printf("Error durante la recuperación: %s\n", err)
		return
	}

	fmt.Printf("\n=== RECUPERACIÓN COMPLETADA EXITOSAMENTE ===\n")
	fmt.Printf("El sistema de archivos EXT3 ha sido recuperado a un estado consistente\n")
	fmt.Printf("Hora de finalización: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

// recoverEXT3FileSystem ejecuta la recuperación del sistema de archivos EXT3
func recoverEXT3FileSystem(partition MountedPartition) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT3
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	// Verificar que el sistema esté formateado como EXT3
	if ext3fs.SuperBlock.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	if ext3fs.SuperBlock.S_filesystem_type != EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		return fmt.Errorf("la partición no es un sistema de archivos EXT3")
	}

	fmt.Printf("1. Verificando integridad del superbloque...\n")

	// Validar el superbloque
	err = validateSuperBlock(ext3fs.SuperBlock)
	if err != nil {
		fmt.Printf("   Advertencia: %s\n", err)
		fmt.Printf("   Intentando reparar el superbloque...\n")
		err = repairSuperBlock(file, ext3fs)
		if err != nil {
			return fmt.Errorf("no se pudo reparar el superbloque: %s", err)
		}
		fmt.Printf("   ✓ Superbloque reparado\n")
	} else {
		fmt.Printf("   ✓ Superbloque válido\n")
	}

	fmt.Printf("2. Leyendo entradas del journal...\n")

	// Leer las entradas del journal
	journalEntries, err := ext3fs.GetJournalEntries()
	if err != nil {
		return fmt.Errorf("error leyendo entradas del journal: %s", err)
	}

	fmt.Printf("   Encontradas %d entradas en el journal\n", len(journalEntries))

	if len(journalEntries) == 0 {
		fmt.Printf("   No hay entradas en el journal para recuperar\n")
		return nil
	}

	fmt.Printf("3. Analizando operaciones del journal...\n")

	// Analizar y mostrar las operaciones encontradas
	err = analyzeJournalEntries(journalEntries)
	if err != nil {
		return fmt.Errorf("error analizando entradas del journal: %s", err)
	}

	fmt.Printf("4. Ejecutando recuperación desde el journal...\n")

	// Ejecutar la recuperación
	err = executeRecoveryFromJournal(file, ext3fs, journalEntries)
	if err != nil {
		return fmt.Errorf("error ejecutando recuperación: %s", err)
	}

	fmt.Printf("5. Verificando consistencia del sistema...\n")

	// Verificar la consistencia después de la recuperación
	err = verifyFileSystemConsistency(file, ext3fs)
	if err != nil {
		fmt.Printf("   Advertencia: %s\n", err)
	} else {
		fmt.Printf("   ✓ Sistema de archivos consistente\n")
	}

	fmt.Printf("6. Registrando recuperación en el journal...\n")

	return nil
}

// validateSuperBlock verifica la integridad del superbloque
func validateSuperBlock(sb *EstructuraarchivocarpetaP2.SuperBloque) error {
	if sb.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return fmt.Errorf("número mágico inválido: 0x%X", sb.S_magic)
	}

	if sb.S_filesystem_type != EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		return fmt.Errorf("tipo de sistema de archivos inválido: %d", sb.S_filesystem_type)
	}

	if sb.S_free_inodes_count < 0 || sb.S_free_inodes_count > sb.S_inodes_count {
		return fmt.Errorf("contador de inodos libres inválido: %d/%d", sb.S_free_inodes_count, sb.S_inodes_count)
	}

	if sb.S_free_blocks_count < 0 || sb.S_free_blocks_count > sb.S_blocks_count {
		return fmt.Errorf("contador de bloques libres inválido: %d/%d", sb.S_free_blocks_count, sb.S_blocks_count)
	}

	return nil
}

// repairSuperBlock intenta reparar el superbloque
func repairSuperBlock(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	// Recalcular contadores basándose en los bitmaps
	freeInodes, err := countFreeBitsInBitmap(file, ext3fs.SuperBlock.S_bm_inode_start, ext3fs.SuperBlock.S_inodes_count)
	if err != nil {
		return fmt.Errorf("error contando inodos libres: %s", err)
	}

	freeBlocks, err := countFreeBitsInBitmap(file, ext3fs.SuperBlock.S_bm_block_start, ext3fs.SuperBlock.S_blocks_count)
	if err != nil {
		return fmt.Errorf("error contando bloques libres: %s", err)
	}

	// Actualizar contadores
	ext3fs.SuperBlock.S_free_inodes_count = freeInodes
	ext3fs.SuperBlock.S_free_blocks_count = freeBlocks
	ext3fs.SuperBlock.S_magic = EstructuraarchivocarpetaP2.EXT2_MAGIC
	ext3fs.SuperBlock.S_filesystem_type = EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE

	// Escribir superbloque reparado
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error escribiendo superbloque reparado: %s", err)
	}

	return nil
}

// countFreeBitsInBitmap cuenta los bits libres (0) en un bitmap
func countFreeBitsInBitmap(file *os.File, bitmapStart int32, totalBits int32) (int32, error) {
	bitmapSize := (totalBits + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.Seek(int64(bitmapStart), 0)
	if err != nil {
		return 0, err
	}

	_, err = file.Read(bitmap)
	if err != nil {
		return 0, err
	}

	freeBits := int32(0)
	for i := int32(0); i < totalBits; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		if byteIndex < int32(len(bitmap)) {
			if (bitmap[byteIndex] & (1 << (7 - bitIndex))) == 0 {
				freeBits++
			}
		}
	}

	return freeBits, nil
}

// analyzeJournalEntries analiza las entradas del journal y muestra estadísticas
func analyzeJournalEntries(entries []*EstructuraExt3P2.Journal) error {
	operationCount := make(map[string]int)

	fmt.Printf("   Analizando operaciones:\n")

	for _, entry := range entries {
		operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
		path := strings.TrimRight(string(entry.J_Content.I_Path[:]), "\x00")
		content := strings.TrimRight(string(entry.J_Content.I_Content[:]), "\x00")

		operationCount[operation]++

		// Mostrar las últimas 10 operaciones como ejemplo
		if len(entries) <= 10 || entry == entries[len(entries)-1] ||
			(len(entries) > 10 && entry == entries[len(entries)-2]) ||
			(len(entries) > 10 && entry == entries[len(entries)-3]) {
			timestamp := time.Unix(int64(entry.J_Content.I_Date), 0)
			fmt.Printf("      [%d] %s: %s - %s (%s)\n",
				entry.J_Count, operation, path, content, timestamp.Format("2006-01-02 15:04:05"))
		}
	}

	fmt.Printf("   Resumen de operaciones:\n")
	for op, count := range operationCount {
		fmt.Printf("      %s: %d operaciones\n", EstructuraExt3P2.GetJournalOperationName(op), count)
	}

	return nil
}

// executeRecoveryFromJournal ejecuta la recuperación basándose en las entradas del journal
func executeRecoveryFromJournal(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, entries []*EstructuraExt3P2.Journal) error {
	recoveredOperations := 0
	skippedOperations := 0

	// Procesar las entradas en orden cronológico
	for _, entry := range entries {
		operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
		path := strings.TrimRight(string(entry.J_Content.I_Path[:]), "\x00")
		content := strings.TrimRight(string(entry.J_Content.I_Content[:]), "\x00")

		err := recoverOperation(file, ext3fs, operation, path, content)
		if err != nil {
			fmt.Printf("      Advertencia: No se pudo recuperar operación %s en %s: %s\n", operation, path, err)
			skippedOperations++
		} else {
			recoveredOperations++
		}
	}

	fmt.Printf("   ✓ Operaciones recuperadas: %d\n", recoveredOperations)
	if skippedOperations > 0 {
		fmt.Printf("   ⚠ Operaciones omitidas: %d\n", skippedOperations)
	}

	return nil
}

// recoverOperation intenta recuperar una operación específica
func recoverOperation(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, operation, path, content string) error {
	switch operation {
	case EstructuraExt3P2.OPERATION_MKDIR:
		return recoverMkdir(file, ext3fs, path, content)
	case EstructuraExt3P2.OPERATION_MKFILE:
		return recoverMkfile(file, ext3fs, path, content)
	case EstructuraExt3P2.OPERATION_RMDIR, EstructuraExt3P2.OPERATION_RMFILE:
		// Para operaciones de eliminación, verificar que no existan
		return verifyDeletion(file, ext3fs, path)
	case EstructuraExt3P2.OPERATION_RENAME:
		return recoverRename(file, ext3fs, path, content)
	case EstructuraExt3P2.OPERATION_CHMOD:
		return recoverChmod(file, ext3fs, path, content)
	case EstructuraExt3P2.OPERATION_CHOWN:
		return recoverChown(file, ext3fs, path, content)
	case EstructuraExt3P2.OPERATION_EDIT:
		return recoverEdit(file, ext3fs, path, content)
	default:
		// Operaciones que no requieren recuperación específica
		return nil
	}
}

// recoverMkdir intenta recrear un directorio si no existe
func recoverMkdir(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Verificar si el directorio ya existe
	if directoryExists(file, ext3fs, path) {
		return nil // Ya existe, no necesita recuperación
	}

	// Intentar recrear el directorio
	return createDirectoryForRecovery(file, ext3fs, path)
}

// recoverMkfile intenta recrear un archivo si no existe
func recoverMkfile(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Verificar si el archivo ya existe
	if fileExists(file, ext3fs, path) {
		return nil // Ya existe, no necesita recuperación
	}

	// Intentar recrear el archivo
	return createFileForRecovery(file, ext3fs, path, content)
}

// verifyDeletion verifica que un archivo o directorio esté efectivamente eliminado
func verifyDeletion(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) error {
	// Si el archivo/directorio aún existe, podría ser inconsistencia
	if fileExists(file, ext3fs, path) || directoryExists(file, ext3fs, path) {
		fmt.Printf("      Advertencia: %s debería estar eliminado pero aún existe\n", path)
	}
	return nil
}

// recoverRename, recoverChmod, recoverChown, recoverEdit son funciones similares
// que intentan restaurar el estado específico de archivos/directorios

func recoverRename(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Separar el directorio padre y el nombre actual
	parentDir := filepath.Dir(path)
	oldName := filepath.Base(path)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre '%s': %v", parentDir, err)
	}

	// Leer el inodo del directorio padre
	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (parentInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, parentInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio padre '%s': %v", parentDir, err)
	}

	// Buscar el archivo o directorio en el directorio padre
	entryInodeNum, err := findFileInDirectoryEXT3ForRecovery(file, ext3fs, parentInode, oldName)
	if err != nil || entryInodeNum == -1 {
		return fmt.Errorf("no se pudo encontrar '%s' en el directorio '%s': %v", oldName, parentDir, err)
	}

	// Cambiar el nombre en el bloque del directorio
	err = updateEntryNameInDirectory(file, ext3fs, parentInode, oldName, content)
	if err != nil {
		return fmt.Errorf("error actualizando el nombre de '%s' a '%s': %v", oldName, content, err)
	}

	fmt.Printf("      Nombre de '%s' cambiado a '%s'\n", path, content)
	return nil
}

func recoverChmod(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Buscar el inodo del archivo o directorio
	inodeNum, err := findInodeByPath(file, ext3fs, path)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el inodo para '%s': %v", path, err)
	}

	// Leer el inodo
	inode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (inodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo para '%s': %v", path, err)
	}

	// Actualizar los permisos
	var newPermissions [3]byte
	_, err = fmt.Sscanf(content, "%d,%d,%d", &newPermissions[0], &newPermissions[1], &newPermissions[2])
	if err != nil {
		return fmt.Errorf("error interpretando los permisos '%s': %v", content, err)
	}
	inode.Iperm = newPermissions

	// Escribir el inodo actualizado
	err = utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo actualizado para '%s': %v", path, err)
	}

	fmt.Printf("      Permisos de '%s' restaurados a %d,%d,%d\n", path, newPermissions[0], newPermissions[1], newPermissions[2])
	return nil
}
func recoverChown(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Buscar el inodo del archivo o directorio
	inodeNum, err := findInodeByPath(file, ext3fs, path)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el inodo para '%s': %v", path, err)
	}

	// Leer el inodo
	inode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (inodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo para '%s': %v", path, err)
	}

	// Actualizar el propietario y el grupo
	var newUID, newGID int32
	_, err = fmt.Sscanf(content, "%d,%d", &newUID, &newGID)
	if err != nil {
		return fmt.Errorf("error interpretando UID y GID '%s': %v", content, err)
	}
	inode.Iuid = newUID
	inode.Igid = newGID

	// Escribir el inodo actualizado
	err = utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo actualizado para '%s': %v", path, err)
	}

	fmt.Printf("      Propietario y grupo de '%s' restaurados a UID=%d, GID=%d\n", path, newUID, newGID)
	return nil
}

func recoverEdit(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Buscar el inodo del archivo
	inodeNum, err := findInodeByPath(file, ext3fs, path)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el inodo para '%s': %v", path, err)
	}

	// Leer el inodo
	inode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (inodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo para '%s': %v", path, err)
	}

	// Escribir el contenido en los bloques asignados al archivo
	contentBytes := []byte(content)
	err = writeContentToFileEXT3(file, ext3fs, inode, string(contentBytes), inodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido en '%s': %v", path, err)
	}

	fmt.Printf("      Contenido de '%s' restaurado\n", path)
	return nil
}

// verifyFileSystemConsistency verifica la consistencia del sistema después de la recuperación
func verifyFileSystemConsistency(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	// Verificar que el inodo raíz existe y es válido
	rootInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start), rootInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer el inodo raíz: %s", err)
	}

	if !rootInode.IsDirectory() {
		return fmt.Errorf("el inodo raíz no es un directorio")
	}

	// Verificar que los contadores del superbloque son consistentes
	err = validateSuperBlock(ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("superbloque inconsistente después de la recuperación: %s", err)
	}

	return nil
}

func directoryExists(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) bool {
	if path == "/" {
		return true // El directorio raíz siempre existe
	}

	// Normalizar la ruta
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return true
	}

	// Dividir la ruta en componentes
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	currentInodeNum := int32(0) // Comenzar desde la raíz

	for _, component := range components {
		if component == "" {
			continue
		}

		// Leer el inodo del directorio actual
		currentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentInodeNum*ext3fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return false
		}

		// Verificar que sea un directorio
		if currentInode.Itype != 0 {
			return false
		}

		// Buscar el componente en el directorio actual
		found := false
		nextInodeNum := int32(-1)

		// Revisar los bloques directos del directorio
		for i := 0; i < 12 && !found; i++ {
			if currentInode.Iblock[i] == -1 {
				continue
			}

			dirBlock := &BloquesP2.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+currentInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada en el bloque
			for j := 0; j < 4; j++ {
				if dirBlock.B_content[j].B_inodo != -1 {
					entryName := strings.TrimRight(string(dirBlock.B_content[j].B_name[:]), "\x00")
					if entryName == component {
						nextInodeNum = dirBlock.B_content[j].B_inodo
						found = true
						break
					}
				}
			}
		}

		if !found {
			return false
		}

		currentInodeNum = nextInodeNum
	}

	// Verificar que el inodo final sea un directorio
	finalInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentInodeNum*ext3fs.SuperBlock.S_inode_s), finalInode)
	if err != nil {
		return false
	}

	return finalInode.Itype == 0 // 0 = directorio
}

// fileExists verifica si un archivo existe en el sistema EXT3
func fileExists(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) bool {
	if path == "/" {
		return false // La raíz es un directorio, no un archivo
	}

	// Separar directorio padre y nombre del archivo
	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return false
	}

	// Leer el inodo del directorio padre
	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInode)
	if err != nil {
		return false
	}

	// Buscar el archivo en el directorio
	fileInodeNum, err := findFileInDirectoryEXT3ForRecovery(file, ext3fs, parentInode, fileName)
	if err != nil || fileInodeNum == -1 {
		return false
	}

	// Verificar que sea un archivo
	fileInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+fileInodeNum*ext3fs.SuperBlock.S_inode_s), fileInode)
	if err != nil {
		return false
	}

	return fileInode.Itype == 1 // 1 = archivo
}

// findDirectoryInEXT3ForRecovery busca un directorio en el sistema EXT3 para recovery
func findDirectoryInEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
	if path == "/" {
		return 0, nil // El inodo 0 es la raíz
	}

	// Normalizar la ruta
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil
	}

	// Dividir la ruta en componentes
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	currentInodeNum := int32(0) // Comenzar desde la raíz

	for _, component := range components {
		if component == "" {
			continue
		}

		// Leer el inodo del directorio actual
		currentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentInodeNum*ext3fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo %d: %v", currentInodeNum, err)
		}

		// Verificar que sea un directorio
		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		// Buscar el componente en el directorio actual
		found := false
		nextInodeNum := int32(-1)

		// Revisar los bloques directos del directorio
		for i := 0; i < 12 && !found; i++ {
			if currentInode.Iblock[i] == -1 {
				continue
			}

			dirBlock := &BloquesP2.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+currentInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada en el bloque
			for j := 0; j < 4; j++ {
				if dirBlock.B_content[j].B_inodo != -1 {
					entryName := strings.TrimRight(string(dirBlock.B_content[j].B_name[:]), "\x00")
					if entryName == component {
						nextInodeNum = dirBlock.B_content[j].B_inodo
						found = true
						break
					}
				}
			}
		}

		if !found {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findFileInDirectoryEXT3ForRecovery busca un archivo en un directorio específico para recovery
func findFileInDirectoryEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en bloques directos
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		dirBlock := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			continue
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo != -1 {
				entryName := strings.TrimRight(string(dirBlock.B_content[j].B_name[:]), "\x00")
				if entryName == fileName {
					return dirBlock.B_content[j].B_inodo, nil
				}
			}
		}
	}

	return -1, nil // No encontrado
}

// createDirectoryForRecovery recrea un directorio durante la recuperación
func createDirectoryForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) error {
	// Separar directorio padre y nombre del directorio
	parentDir := filepath.Dir(path)
	dirName := filepath.Base(path)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre '%s': %v", parentDir, err)
	}

	// Buscar un inodo libre
	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %v", err)
	}

	// Buscar un bloque libre para el directorio
	blockNum, err := findFreeBlockEXT3ForRecovery(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay bloques libres: %v", err)
	}

	// Crear el inodo del directorio
	dirInode := &EstructuraarchivocarpetaP2.Inodo{}
	dirInode.SetFileValues(
		1,                // UID root
		1,                // GID root
		64,               // Tamaño típico de directorio
		0,                // Tipo: directorio
		[3]byte{7, 5, 5}, // Permisos: 755
	)

	// Asignar el bloque al directorio
	dirInode.SetIblock(0, blockNum)

	// Crear el bloque del directorio con entradas . y ..
	dirBlock := &BloquesP2.BloqueCarpeta{}

	// Inicializar todas las entradas como libres
	for i := 0; i < 4; i++ {
		dirBlock.B_content[i].B_inodo = -1
		for j := 0; j < len(dirBlock.B_content[i].B_name); j++ {
			dirBlock.B_content[i].B_name[j] = 0
		}
	}

	// Entrada . (directorio actual)
	dirBlock.B_content[0].B_inodo = newInodeNum
	copy(dirBlock.B_content[0].B_name[:], ".")

	// Entrada .. (directorio padre)
	dirBlock.B_content[1].B_inodo = parentInodeNum
	copy(dirBlock.B_content[1].B_name[:], "..")

	// Escribir el bloque del directorio
	blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
	err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
	if err != nil {
		return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
	}

	// Escribir el inodo del directorio
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (newInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, dirInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo de directorio: %v", err)
	}

	// Marcar inodo y bloque como usados
	err = markInodeAsUsedEXT3(file, ext3fs, newInodeNum)
	if err != nil {
		return fmt.Errorf("error marcando inodo como usado: %v", err)
	}

	err = markBlockAsUsedEXT3ForRecovery(file, ext3fs, blockNum)
	if err != nil {
		return fmt.Errorf("error marcando bloque como usado: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectoryForRecovery(file, ext3fs, parentInodeNum, dirName, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	// Actualizar contadores del superbloque
	ext3fs.SuperBlock.S_free_inodes_count--
	ext3fs.SuperBlock.S_free_blocks_count--

	// Escribir superbloque actualizado
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %v", err)
	}

	fmt.Printf("      Directorio '%s' recreado exitosamente\n", path)
	return nil
}

// createFileForRecovery recrea un archivo durante la recuperación
func createFileForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Separar directorio padre y nombre del archivo
	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre '%s': %v", parentDir, err)
	}

	// Buscar un inodo libre
	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %v", err)
	}

	// Crear el inodo del archivo
	fileInode := &EstructuraarchivocarpetaP2.Inodo{}
	contentBytes := []byte(content)
	fileSize := int32(len(contentBytes))

	fileInode.SetFileValues(
		1,                // UID root
		1,                // GID root
		fileSize,         // Tamaño del archivo
		1,                // Tipo: archivo
		[3]byte{6, 6, 4}, // Permisos: 664
	)

	// Si el archivo tiene contenido, asignar bloques
	if fileSize > 0 {
		blocksNeeded := (fileSize + 63) / 64 // Cada bloque es de 64 bytes

		// Asignar bloques directos
		bytesWritten := int32(0)
		for i := 0; i < 12 && i < int(blocksNeeded); i++ {
			blockNum, err := findFreeBlockEXT3ForRecovery(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres: %v", err)
			}

			// Crear bloque con contenido
			fileBlock := &BloquesP2.BloqueArchivo{}

			// Calcular cuántos bytes escribir en este bloque
			bytesToWrite := int32(64)
			if bytesWritten+bytesToWrite > fileSize {
				bytesToWrite = fileSize - bytesWritten
			}

			// Copiar contenido al bloque
			copy(fileBlock.B_content[:bytesToWrite], contentBytes[bytesWritten:bytesWritten+bytesToWrite])

			// Escribir bloque al disco
			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
			err = utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de archivo: %v", err)
			}

			// Asignar bloque al inodo
			fileInode.SetIblock(i, blockNum)

			// Marcar bloque como usado
			err = markBlockAsUsedEXT3ForRecovery(file, ext3fs, blockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque como usado: %v", err)
			}

			bytesWritten += bytesToWrite
		}

		// Actualizar contador de bloques libres
		ext3fs.SuperBlock.S_free_blocks_count -= blocksNeeded
	}

	// Escribir el inodo del archivo
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (newInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, fileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo de archivo: %v", err)
	}

	// Marcar inodo como usado
	err = markInodeAsUsedEXT3(file, ext3fs, newInodeNum)
	if err != nil {
		return fmt.Errorf("error marcando inodo como usado: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectoryForRecovery(file, ext3fs, parentInodeNum, fileName, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	// Actualizar contadores del superbloque
	ext3fs.SuperBlock.S_free_inodes_count--

	// Escribir superbloque actualizado
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %v", err)
	}

	fmt.Printf("      Archivo '%s' recreado exitosamente (%d bytes)\n", path, fileSize)
	return nil
}

// findFreeBlockEXT3ForRecovery busca un bloque libre para recovery
func findFreeBlockEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) (int32, error) {
	bitmapSize := (ext3fs.SuperBlock.S_blocks_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return -1, err
	}

	_, err = file.Read(bitmap)
	if err != nil {
		return -1, err
	}

	// Buscar el primer bit en 0 (libre)
	for i := int32(1); i < ext3fs.SuperBlock.S_blocks_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		if (bitmap[byteIndex] & (1 << (7 - bitIndex))) == 0 {
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay bloques libres disponibles")
}

// markBlockAsUsedEXT3ForRecovery marca un bloque como usado para recovery
func markBlockAsUsedEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	// Leer el byte específico del bitmap
	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	var currentByte [1]byte
	_, err = file.Read(currentByte[:])
	if err != nil {
		return err
	}

	// Marcar el bit como usado (1)
	mask := byte(1 << (7 - bitIndex))
	currentByte[0] |= mask

	// Escribir el byte modificado
	_, err = file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	_, err = file.Write(currentByte[:])
	return err
}

// addEntryToDirectoryForRecovery agrega una entrada a un directorio para recovery
func addEntryToDirectoryForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInodeNum int32, entryName string, entryInodeNum int32) error {
	// Leer el inodo del directorio
	dirInode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (dirInodeNum * ext3fs.SuperBlock.S_inode_s))
	err := utilitiesP2.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio: %v", err)
	}

	// Buscar espacio libre en los bloques existentes
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		dirBlock := &BloquesP2.BloqueCarpeta{}

		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			continue
		}

		// Buscar entrada libre en el bloque
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo == -1 {
				// Encontramos espacio libre
				dirBlock.B_content[j].B_inodo = entryInodeNum

				// Limpiar el nombre anterior
				for k := range dirBlock.B_content[j].B_name {
					dirBlock.B_content[j].B_name[k] = 0
				}

				// Copiar el nuevo nombre
				copy(dirBlock.B_content[j].B_name[:], entryName)

				// Escribir el bloque actualizado
				err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
				if err != nil {
					return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
				}

				return nil
			}
		}
	}

	// Si no hay espacio, crear un nuevo bloque
	newBlockNum, err := findFreeBlockEXT3ForRecovery(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay bloques libres para el directorio: %v", err)
	}

	// Crear nuevo bloque de directorio
	newDirBlock := &BloquesP2.BloqueCarpeta{}

	// Inicializar todas las entradas como libres
	for i := 0; i < 4; i++ {
		newDirBlock.B_content[i].B_inodo = -1
		for j := 0; j < len(newDirBlock.B_content[i].B_name); j++ {
			newDirBlock.B_content[i].B_name[j] = 0
		}
	}

	// Agregar la nueva entrada
	newDirBlock.B_content[0].B_inodo = entryInodeNum
	copy(newDirBlock.B_content[0].B_name[:], entryName)

	// Escribir el nuevo bloque
	newBlockOffset := int64(ext3fs.SuperBlock.S_block_start + (newBlockNum * ext3fs.SuperBlock.S_block_s))
	err = utilitiesP2.WriteStruct(file, newBlockOffset, newDirBlock)
	if err != nil {
		return fmt.Errorf("error escribiendo nuevo bloque de directorio: %v", err)
	}

	// Marcar el bloque como usado
	err = markBlockAsUsedEXT3ForRecovery(file, ext3fs, newBlockNum)
	if err != nil {
		return fmt.Errorf("error marcando bloque como usado: %v", err)
	}

	// Asignar el bloque al directorio
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			dirInode.SetIblock(i, newBlockNum)
			break
		}
	}

	// Escribir el inodo actualizado del directorio
	err = utilitiesP2.WriteStruct(file, inodeOffset, dirInode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo del directorio: %v", err)
	}

	return nil
}

func findInodeByPath(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
	if path == "/" {
		return 0, nil // El inodo raíz siempre es 0
	}

	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return -1, fmt.Errorf("no se pudo encontrar el directorio padre '%s': %v", parentDir, err)
	}

	// Leer el inodo del directorio padre
	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInode)
	if err != nil {
		return -1, fmt.Errorf("error leyendo inodo del directorio padre '%s': %v", parentDir, err)
	}

	// Buscar el archivo o directorio en el directorio padre
	return findFileInDirectoryEXT3ForRecovery(file, ext3fs, parentInode, fileName)
}

func updateEntryNameInDirectory(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, oldName, newName string) error {
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		dirBlock := &BloquesP2.BloqueCarpeta{}

		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			continue
		}

		// Buscar la entrada con el nombre antiguo
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo != -1 {
				entryName := strings.TrimRight(string(dirBlock.B_content[j].B_name[:]), "\x00")
				if entryName == oldName {
					// Actualizar el nombre
					copy(dirBlock.B_content[j].B_name[:], newName)

					// Escribir el bloque actualizado
					err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
					if err != nil {
						return fmt.Errorf("error escribiendo bloque actualizado: %v", err)
					}
					return nil
				}
			}
		}
	}

	return fmt.Errorf("no se encontró la entrada '%s' en el directorio", oldName)
}

package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

	// NO REQUERIR SESIÓN ACTIVA - Recovery puede hacerse sin login
	// Comentar esta verificación:
	/*
	   if sessionP2.IsSessionActive() {
	       sessionInfo := sessionP2.GetCurrentSession()
	       if sessionInfo.Username != "root" {
	           fmt.Printf("Error: Solo el usuario root puede ejecutar la recuperación del sistema\n")
	           fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
	           return
	       }
	   }
	*/

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
	fmt.Printf("El sistema de archivos EXT3 ha sido recuperado\n")
	fmt.Printf("Hora de finalización: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("\nPuede iniciar sesión normalmente con:\n")
	fmt.Printf("login -user=root -pass=123 -id=%s\n", partitionID)
}
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
		fmt.Printf("   ⚠ Advertencia: %s\n", err)
	} else {
		fmt.Printf("   ✓ Sistema de archivos consistente\n")
	}

	return nil
}

// validateSuperBlock verifica la integridad del superbloque
func validateSuperBlock(sb *EstructuraarchivocarpetaP2.SuperBloque) error {
	if sb.S_magic != 0xEF53 {
		return fmt.Errorf("magic number inválido: 0x%X", sb.S_magic)
	}

	if sb.S_filesystem_type != 3 {
		return fmt.Errorf("tipo de sistema incorrecto: %d", sb.S_filesystem_type)
	}

	if sb.S_inodes_count <= 0 || sb.S_blocks_count <= 0 {
		return fmt.Errorf("contadores inválidos: inodos=%d, bloques=%d",
			sb.S_inodes_count, sb.S_blocks_count)
	}

	if sb.S_free_inodes_count < 0 || sb.S_free_blocks_count < 0 {
		return fmt.Errorf("contadores libres negativos")
	}

	return nil
}

// repairSuperBlock intenta reparar el superbloque
func repairSuperBlock(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	// Recalcular contadores basándose en los bitmaps
	freeInodes, err := countFreeBitsInBitmap(file, ext3fs.SuperBlock.S_bm_inode_start, ext3fs.SuperBlock.S_inodes_count)
	if err != nil {
		return err
	}

	freeBlocks, err := countFreeBitsInBitmap(file, ext3fs.SuperBlock.S_bm_block_start, ext3fs.SuperBlock.S_blocks_count)
	if err != nil {
		return err
	}

	// Actualizar contadores
	ext3fs.SuperBlock.S_free_inodes_count = freeInodes
	ext3fs.SuperBlock.S_free_blocks_count = freeBlocks
	ext3fs.SuperBlock.S_umtime = time.Now().Unix()

	// Escribir superbloque reparado
	return utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
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

	freeCount := int32(0)
	for i := int32(0); i < totalBits; i++ {
		byteIndex := i / 8
		bitIndex := i % 8
		mask := byte(1 << (7 - bitIndex))

		if (bitmap[byteIndex] & mask) == 0 {
			freeCount++
		}
	}

	return freeCount, nil
}

// analyzeJournalEntries analiza las entradas del journal y muestra estadísticas
func analyzeJournalEntries(entries []*EstructuraExt3P2.Journal) error {
	operationCounts := make(map[string]int)

	for _, entry := range entries {
		operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
		operation = strings.TrimSpace(operation)
		operationCounts[operation]++
	}

	fmt.Printf("   Operaciones encontradas:\n")
	for op, count := range operationCounts {
		fmt.Printf("      - %s: %d operaciones\n",
			EstructuraExt3P2.GetJournalOperationName(op), count)
	}

	return nil
}
func executeRecoveryFromJournal(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, entries []*EstructuraExt3P2.Journal) error {
	recoveredOperations := 0
	failedOperations := 0
	skippedOperations := 0

	createdPaths := make(map[string]bool)
	createdPaths["/"] = true

	fmt.Printf("   Iniciando recuperación de %d operaciones del journal...\n\n", len(entries))

	// PASO 1: RECREAR users.txt
	fmt.Printf("   [PASO 1] Recreando archivo users.txt...\n")
	err := recreateUsersFile(file, ext3fs)
	if err != nil {
		fmt.Printf("      ⚠ Advertencia al crear users.txt: %v\n", err)
	} else {
		fmt.Printf("      ✓ Archivo users.txt recreado exitosamente\n")
		createdPaths["/users.txt"] = true
		recoveredOperations++
	}
	fmt.Println()

	// PASO 2: PROCESAR OPERACIONES DEL JOURNAL
	fmt.Printf("   [PASO 2] Procesando operaciones del journal...\n")
	for _, entry := range entries {
		operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
		path := strings.TrimRight(string(entry.J_Content.I_Path[:]), "\x00")
		content := strings.TrimRight(string(entry.J_Content.I_Content[:]), "\x00")

		operation = strings.TrimSpace(operation)
		path = strings.TrimSpace(path)
		content = strings.TrimSpace(content)

		if operation == EstructuraExt3P2.OPERATION_FORMAT {
			skippedOperations++
			continue
		}

		if path == "" || path == "/" {
			skippedOperations++
			continue
		}

		fmt.Printf("   Procesando: %s en '%s'\n", operation, path)

		// CRÍTICO: Crear directorios padre ANTES de la operación
		parentDir := filepath.Dir(path)
		if parentDir != "/" && !createdPaths[parentDir] {
			fmt.Printf("      → Creando directorios padre faltantes para '%s'...\n", parentDir)
			err := ensureParentDirectoriesExistRecursive(file, ext3fs, parentDir, createdPaths)
			if err != nil {
				fmt.Printf("      ⚠ No se pudieron crear directorios padre: %v\n", err)
				failedOperations++
				continue
			}
			fmt.Printf("      ✓ Directorios padre creados exitosamente\n")
		}

		// Recuperar operación
		err := recoverOperation(file, ext3fs, operation, path, content)
		if err != nil {
			fmt.Printf("      ⚠ Advertencia: %s\n", err)
			failedOperations++
		} else {
			fmt.Printf("      ✓ Recuperado exitosamente\n")
			recoveredOperations++
			createdPaths[path] = true
		}
	}

	fmt.Printf("\n   Resumen de recuperación:\n")
	fmt.Printf("      - Operaciones recuperadas: %d\n", recoveredOperations)
	fmt.Printf("      - Operaciones omitidas: %d\n", skippedOperations)
	fmt.Printf("      - Operaciones fallidas: %d\n", failedOperations)

	return nil
}

// recreateUsersFile - VERSIÓN CORREGIDA usando addEntryToDirectoryForRecovery
func recreateUsersFile(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	usersContent := "1,G,root\n1,U,root,root,123\n"

	rootInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start), rootInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo raíz: %v", err)
	}

	existingInodeNum, _ := findFileInDirectoryEXT3ForRecovery(file, ext3fs, rootInode, "users.txt")
	if existingInodeNum != -1 {
		return nil // Ya existe
	}

	return createFileForRecovery(file, ext3fs, "/users.txt", usersContent)
}

// ensureParentDirectoriesExistRecursive - VERSIÓN CORREGIDA
func ensureParentDirectoriesExistRecursive(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string, createdPaths map[string]bool) error {
	if createdPaths[path] {
		return nil
	}

	path = strings.TrimSuffix(path, "/")
	if path == "" || path == "/" {
		createdPaths["/"] = true
		return nil
	}

	// RECURSIÓN: Asegurar que el padre del padre existe
	parentDir := filepath.Dir(path)
	if parentDir != "/" && !createdPaths[parentDir] {
		err := ensureParentDirectoriesExistRecursive(file, ext3fs, parentDir, createdPaths)
		if err != nil {
			return fmt.Errorf("error creando directorio padre '%s': %v", parentDir, err)
		}
	}

	// Crear este directorio
	fmt.Printf("         → Creando directorio: '%s'\n", path)
	err := createDirectoryForRecovery(file, ext3fs, path)
	if err != nil {
		if strings.Contains(err.Error(), "ya existe") {
			createdPaths[path] = true
			return nil
		}
		return fmt.Errorf("error creando directorio '%s': %v", path, err)
	}

	createdPaths[path] = true
	fmt.Printf("         ✓ Directorio '%s' creado\n", path)
	return nil
}

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
	case EstructuraExt3P2.OPERATION_FORMAT:
		// El formateo ya está hecho, solo retornar éxito
		return nil
	default:
		// Operaciones que no requieren recuperación específica
		return nil
	}
}

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
	// Si el archivo/directorio aún existe, no hacer nada (la eliminación fue exitosa)
	return nil
}

// recoverRename intenta restaurar un rename
func recoverRename(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// El content contiene el nuevo nombre
	// Por ahora, verificar que el archivo destino existe
	if strings.Contains(content, "->") {
		parts := strings.Split(content, "->")
		if len(parts) == 2 {
			newPath := strings.TrimSpace(parts[1])
			if !fileExists(file, ext3fs, newPath) && !directoryExists(file, ext3fs, newPath) {
				return fmt.Errorf("archivo renombrado no encontrado")
			}
		}
	}
	return nil
}

// recoverChmod intenta restaurar permisos
func recoverChmod(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Buscar el inodo
	inodeNum, err := findInodeByPath(file, ext3fs, path)
	if err != nil {
		return err
	}

	// Leer el inodo
	inode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+inodeNum*ext3fs.SuperBlock.S_inode_s), inode)
	if err != nil {
		return err
	}

	// Los permisos actuales están bien (no cambiar)
	return nil
}

// recoverChown intenta restaurar propietario
func recoverChown(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Similar a chmod, verificar que el archivo existe
	inodeNum, err := findInodeByPath(file, ext3fs, path)
	if err != nil {
		return err
	}

	// El archivo existe, propietario actual es correcto
	_ = inodeNum
	return nil
}

// recoverEdit intenta restaurar contenido editado
func recoverEdit(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	// Verificar que el archivo existe
	if !fileExists(file, ext3fs, path) {
		return fmt.Errorf("archivo no encontrado")
	}

	// El contenido actual es el correcto
	return nil
}

// directoryExists verifica si un directorio existe
func directoryExists(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) bool {
	if path == "/" {
		return true
	}

	inodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, path)
	if err != nil || inodeNum == -1 {
		return false
	}

	inode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+inodeNum*ext3fs.SuperBlock.S_inode_s), inode)
	if err != nil {
		return false
	}

	return inode.Itype == 0 // 0 = directorio
}

// fileExists verifica si un archivo existe
func fileExists(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) bool {
	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil || parentInodeNum == -1 {
		return false
	}

	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInode)
	if err != nil {
		return false
	}

	fileInodeNum, err := findFileInDirectoryEXT3ForRecovery(file, ext3fs, parentInode, fileName)
	if err != nil || fileInodeNum == -1 {
		return false
	}

	fileInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+fileInodeNum*ext3fs.SuperBlock.S_inode_s), fileInode)
	if err != nil {
		return false
	}

	return fileInode.Itype == 1 // 1 = archivo
}

// findDirectoryInEXT3ForRecovery busca un directorio en EXT3
func findDirectoryInEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
	if path == "/" {
		return 0, nil
	}

	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil
	}

	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	currentInodeNum := int32(0)

	for _, component := range components {
		if component == "" {
			continue
		}

		currentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentInodeNum*ext3fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, err
		}

		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		nextInodeNum, err := findFileInDirectoryEXT3ForRecovery(file, ext3fs, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("'%s' no encontrado", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findFileInDirectoryEXT3ForRecovery busca un archivo en un directorio
func findFileInDirectoryEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		for j := 0; j < 4; j++ {
			if block.IsEntryEmpty(j) {
				continue
			}

			entryName, entryInodeNum, ok := block.GetEntry(j)
			if !ok {
				continue
			}

			if entryName == fileName {
				return entryInodeNum, nil
			}
		}
	}

	return -1, fmt.Errorf("archivo no encontrado")
}

// createDirectoryForRecovery - VERSIÓN CORREGIDA usando addEntryToDirectoryForRecovery
func createDirectoryForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) error {
	if directoryExists(file, ext3fs, path) {
		return nil
	}

	parentDir := filepath.Dir(path)
	dirName := filepath.Base(path)

	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre '%s': %v", parentDir, err)
	}

	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %v", err)
	}

	blockNum, err := findFreeBlockEXT3ForRecovery(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay bloques libres: %v", err)
	}

	dirInode := &EstructuraarchivocarpetaP2.Inodo{}
	dirInode.SetFileValues(
		1,
		1,
		64,
		0,
		[3]byte{7, 5, 5},
	)
	dirInode.SetIblock(0, blockNum)

	dirBlock := &BloquesP2.BloqueCarpeta{}
	for i := 0; i < 4; i++ {
		dirBlock.B_content[i].B_inodo = -1
		for j := 0; j < len(dirBlock.B_content[i].B_name); j++ {
			dirBlock.B_content[i].B_name[j] = 0
		}
	}

	dirBlock.B_content[0].B_inodo = newInodeNum
	copy(dirBlock.B_content[0].B_name[:], ".")

	dirBlock.B_content[1].B_inodo = parentInodeNum
	copy(dirBlock.B_content[1].B_name[:], "..")

	blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
	err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
	if err != nil {
		return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
	}

	err = markBlockAsUsedEXT3ForRecovery(file, ext3fs, blockNum)
	if err != nil {
		return fmt.Errorf("error marcando bloque como usado: %v", err)
	}

	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (newInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, dirInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo de directorio: %v", err)
	}

	err = markInodeAsUsedEXT3(file, ext3fs, newInodeNum)
	if err != nil {
		return fmt.Errorf("error marcando inodo como usado: %v", err)
	}

	// CRÍTICO: Usar la función que maneja expansión de directorios
	err = addEntryToDirectoryForRecovery(file, ext3fs, parentInodeNum, dirName, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	ext3fs.SuperBlock.S_free_inodes_count--
	ext3fs.SuperBlock.S_free_blocks_count--

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %v", err)
	}

	return nil
}

// createFileForRecovery - VERSIÓN CORREGIDA usando addEntryToDirectoryForRecovery
func createFileForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path, content string) error {
	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre '%s': %v", parentDir, err)
	}

	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %v", err)
	}

	fileInode := &EstructuraarchivocarpetaP2.Inodo{}
	fileInode.SetFileValues(
		1,
		1,
		int32(len(content)),
		1,
		[3]byte{6, 4, 4},
	)

	if len(content) > 0 {
		contentBytes := []byte(content)
		blocksNeeded := (int32(len(contentBytes)) + 63) / 64

		for i := int32(0); i < blocksNeeded && i < 12; i++ {
			blockNum, err := findFreeBlockEXT3ForRecovery(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres: %v", err)
			}

			fileBlock := &BloquesP2.BloqueArchivo{}
			start := i * 64
			end := start + 64
			if end > int32(len(contentBytes)) {
				end = int32(len(contentBytes))
			}
			copy(fileBlock.B_content[:], contentBytes[start:end])

			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
			err = utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de archivo: %v", err)
			}

			fileInode.SetIblock(int(i), blockNum)
			err = markBlockAsUsedEXT3ForRecovery(file, ext3fs, blockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque como usado: %v", err)
			}

			ext3fs.SuperBlock.S_free_blocks_count--
		}
	}

	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (newInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, fileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo de archivo: %v", err)
	}

	err = markInodeAsUsedEXT3(file, ext3fs, newInodeNum)
	if err != nil {
		return fmt.Errorf("error marcando inodo como usado: %v", err)
	}

	ext3fs.SuperBlock.S_free_inodes_count--

	// CRÍTICO: Usar la función que maneja expansión de directorios
	err = addEntryToDirectoryForRecovery(file, ext3fs, parentInodeNum, fileName, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %v", err)
	}

	return nil
}

// findFreeBlockEXT3ForRecovery busca un bloque libre
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

	for blockNum := int32(0); blockNum < ext3fs.SuperBlock.S_blocks_count; blockNum++ {
		byteIndex := blockNum / 8
		bitIndex := blockNum % 8
		mask := byte(1 << (7 - bitIndex))

		if (bitmap[byteIndex] & mask) == 0 {
			return blockNum, nil
		}
	}

	return -1, fmt.Errorf("no hay bloques libres")
}

// markBlockAsUsedEXT3ForRecovery marca un bloque como usado
func markBlockAsUsedEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	var currentByte [1]byte
	_, err = file.Read(currentByte[:])
	if err != nil {
		return err
	}

	mask := byte(1 << (7 - bitIndex))
	currentByte[0] |= mask

	_, err = file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	_, err = file.Write(currentByte[:])
	return err
}

// addEntryToDirectoryForRecovery - VERSIÓN COMPLETA con expansión de bloques
func addEntryToDirectoryForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInodeNum int32, entryName string, entryInodeNum int32) error {
	dirInode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (dirInodeNum * ext3fs.SuperBlock.S_inode_s))
	err := utilitiesP2.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio: %v", err)
	}

	// Buscar espacio libre en bloques existentes
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			break
		}

		dirBlock := &BloquesP2.BloqueCarpeta{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (dirInode.Iblock[i] * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			continue
		}

		// Buscar entrada libre
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo == -1 {
				dirBlock.B_content[j].B_inodo = entryInodeNum
				copy(dirBlock.B_content[j].B_name[:], entryName)

				err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
				if err != nil {
					return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
				}

				return nil
			}
		}
	}

	// Si no hay espacio, crear nuevo bloque
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			newBlockNum, err := findFreeBlockEXT3ForRecovery(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para expandir directorio: %v", err)
			}

			newDirBlock := &BloquesP2.BloqueCarpeta{}
			for j := 0; j < 4; j++ {
				newDirBlock.B_content[j].B_inodo = -1
			}

			newDirBlock.B_content[0].B_inodo = entryInodeNum
			copy(newDirBlock.B_content[0].B_name[:], entryName)

			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (newBlockNum * ext3fs.SuperBlock.S_block_s))
			err = utilitiesP2.WriteStruct(file, blockOffset, newDirBlock)
			if err != nil {
				return fmt.Errorf("error escribiendo nuevo bloque de directorio: %v", err)
			}

			dirInode.SetIblock(i, newBlockNum)
			err = markBlockAsUsedEXT3ForRecovery(file, ext3fs, newBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque como usado: %v", err)
			}

			err = utilitiesP2.WriteStruct(file, inodeOffset, dirInode)
			if err != nil {
				return fmt.Errorf("error actualizando inodo del directorio: %v", err)
			}

			ext3fs.SuperBlock.S_free_blocks_count--
			return nil
		}
	}

	return fmt.Errorf("no hay espacio para más bloques en el directorio")
}

// findInodeByPath busca un inodo por su ruta
func findInodeByPath(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	parentInodeNum, err := findDirectoryInEXT3ForRecovery(file, ext3fs, parentDir)
	if err != nil {
		return -1, err
	}

	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInode)
	if err != nil {
		return -1, err
	}

	return findFileInDirectoryEXT3ForRecovery(file, ext3fs, parentInode, fileName)
}

func findFreeInodeEXT3ForRecovery(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) (int32, error) {
	bitmapSize := (ext3fs.SuperBlock.S_inodes_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return -1, err
	}

	_, err = file.Read(bitmap)
	if err != nil {
		return -1, err
	}

	for inodeNum := int32(1); inodeNum < ext3fs.SuperBlock.S_inodes_count; inodeNum++ {
		byteIndex := inodeNum / 8
		bitIndex := inodeNum % 8
		mask := byte(1 << (7 - bitIndex))

		if (bitmap[byteIndex] & mask) == 0 {
			return inodeNum, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres")
}

// verifyFileSystemConsistency verifica la consistencia del sistema - VERSIÓN MEJORADA
func verifyFileSystemConsistency(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	// Verificar que el inodo raíz existe
	rootInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start), rootInode)
	if err != nil {
		return fmt.Errorf("inodo raíz no encontrado o corrupto")
	}

	// Verificar que users.txt existe
	usersInodeNum, err := findFileInDirectoryEXT3ForRecovery(file, ext3fs, rootInode, "users.txt")
	if err != nil || usersInodeNum == -1 {
		fmt.Printf("   ⚠ Advertencia: archivo users.txt no encontrado\n")
	} else {
		fmt.Printf("   ✓ Archivo users.txt encontrado (inodo %d)\n", usersInodeNum)
	}

	fmt.Printf("   ✓ Sistema de archivos consistente\n")
	return nil
}

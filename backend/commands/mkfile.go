package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilities "MIA_2S2025_P2_202307820/utilities"
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MKFILE implementa el comando para crear archivos
func MKFILE(args []string) {
	// Verificar que hay una sesión activa
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		return
	}

	// PARSEAR PARÁMETROS DE FORMA SIMPLIFICADA
	params := utilities.ParseParameters(args)

	// EXTRAER Y VALIDAR PARÁMETROS
	filePath := params["path"]
	sizeStr := params["size"]
	contentFile := params["cont"]
	createParents := utilities.HasParam(params, "r")

	// Validar parámetros obligatorios
	if filePath == "" {
		fmt.Println("Error: Falta el parámetro obligatorio -path")
		fmt.Println("Uso: mkfile -path=<ruta> [-size=<bytes>] [-cont=<archivo>] [-r]")
		return
	}

	// Limpiar comillas si las tiene
	filePath = strings.Trim(filePath, "\"")
	contentFile = strings.Trim(contentFile, "\"")

	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(filePath, "/") {
		fmt.Println("Error: La ruta debe ser absoluta (comenzar con /)")
		return
	}

	// PARSEAR SIZE DE FORMA SIMPLE
	var size int32 = 0
	var hasSize bool = false
	if sizeStr != "" {
		sizeVal, err := strconv.ParseInt(sizeStr, 10, 32)
		if err != nil {
			fmt.Printf("Error: Valor de size inválido: %s\n", sizeStr)
			return
		}
		if sizeVal < 0 {
			fmt.Printf("Error: El tamaño no puede ser negativo: %d\n", sizeVal)
			return
		}
		size = int32(sizeVal)
		hasSize = true
	}

	// VERIFICAR SI HAY CONTENIDO
	hasContent := contentFile != ""

	// VALIDACIÓN ESPECÍFICA: -r no debe tener valor
	if rValue, exists := params["r"]; exists && rValue != "" {
		fmt.Println("Error: El parámetro -r no debe recibir ningún valor")
		return
	}

	// Obtener información de la sesión
	sessionInfo := session.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Println("Error: No se pudo obtener información de la sesión")
		return
	}

	// Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	fsTypes, err := getFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	if fsTypes == estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		err = createFile(mountedPartition, filePath, createParents, size, contentFile, hasSize, hasContent, sessionInfo)
	} else if fsTypes == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		err = createFileEXT3(mountedPartition, filePath, createParents, size, contentFile, hasSize, hasContent, sessionInfo)
	}

	if err != nil {
		fmt.Printf("Error creando archivo: %s\n", err)
		return
	}

	fmt.Printf("Archivo '%s' creado exitosamente\n", filePath)
}

// createFile crea un archivo en el sistema EXT2
func createFile(partition MountedPartition, filePath string, createParents bool, size int32, contentFile string, hasSize, hasContent bool, sessionInfo *session.UserSession) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT2
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no está formateado como EXT2")
	}

	// Separar directorio padre y nombre del archivo
	parentDir, fileName := filepath.Split(filePath)
	parentDir = strings.TrimSuffix(parentDir, "/")
	if parentDir == "" {
		parentDir = "/"
	}

	fmt.Printf("DEBUG: Creando archivo '%s' en directorio '%s'\n", fileName, parentDir)

	// Buscar o crear el directorio padre
	parentInodeNum, err := findOrCreateDirectory(file, ext2fs, parentDir, createParents, sessionInfo)
	if err != nil {
		return fmt.Errorf("error con directorio padre: %s", err)
	}

	// Leer el inodo del directorio padre
	parentInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (parentInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilities.ReadStruct(file, inodeOffset, parentInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio padre: %s", err)
	}

	// Verificar permisos de escritura en el directorio padre
	if !session.IsRootUser() {
		if !session.CanWrite(parentInode) {
			return fmt.Errorf("sin permisos de escritura en el directorio padre '%s'", parentDir)
		}
	}

	// Verificar si el archivo ya existe
	existingInodeNum, _ := findFileInDirectory(file, ext2fs, parentInodeNum, fileName)
	if existingInodeNum != -1 {
		fmt.Printf("El archivo '%s' ya existe. ¿Desea sobreescribirlo? (s/N): ", filePath)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))

		if response != "s" && response != "sí" && response != "si" && response != "yes" && response != "y" {
			return fmt.Errorf("operación cancelada por el usuario")
		}

		// Eliminar el archivo existente
		err = deleteFileFromDirectory(file, ext2fs, parentInodeNum, fileName, existingInodeNum)
		if err != nil {
			return fmt.Errorf("error eliminando archivo existente: %s", err)
		}
	}

	// Obtener el contenido del archivo
	var fileContent []byte
	var actualSize int32

	if hasContent {
		fileContent, err = ioutil.ReadFile(contentFile)
		if err != nil {
			return fmt.Errorf("error leyendo archivo de contenido '%s': %s", contentFile, err)
		}
		actualSize = int32(len(fileContent))
		fmt.Printf("Contenido cargado desde '%s': %d bytes\n", contentFile, actualSize)
	} else if hasSize {
		fileContent = generateNumberContent(size)
		actualSize = size
		fmt.Printf("Contenido generado: %d bytes (números 0-9)\n", actualSize)
	} else {
		fileContent = []byte{}
		actualSize = 0
		fmt.Printf("Archivo vacío creado\n")
	}

	// Crear el archivo en el sistema EXT2
	err = createFileInEXT2System(file, ext2fs, parentInodeNum, fileName, fileContent, actualSize, sessionInfo, partition)
	if err != nil {
		return fmt.Errorf("error creando archivo en sistema EXT2: %s", err)
	}

	return nil
}

func createFileEXT3(partition MountedPartition, filePath string, createParents bool, size int32, contentFile string, hasSize, hasContent bool, sessionInfo *session.UserSession) error {
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

	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no está formateado como EXT3")
	}

	// Separar directorio padre y nombre del archivo
	parentDir, fileName := filepath.Split(filePath)
	parentDir = strings.TrimSuffix(parentDir, "/")
	if parentDir == "" {
		parentDir = "/"
	}

	fmt.Printf("DEBUG: Creando archivo '%s' en directorio '%s'\n", fileName, parentDir)

	// Buscar o crear el directorio padre
	parentInodeNum, err := findOrCreateDirectoryEXT3(file, ext3fs, parentDir, createParents, sessionInfo)
	if err != nil {
		return fmt.Errorf("error con directorio padre: %s", err)
	}

	// Leer el inodo del directorio padre
	parentInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (parentInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilities.ReadStruct(file, inodeOffset, parentInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio padre: %s", err)
	}

	// Verificar permisos de escritura en el directorio padre
	if !session.IsRootUser() {
		if !session.CanWrite(parentInode) {
			return fmt.Errorf("sin permisos de escritura en el directorio padre '%s'", parentDir)
		}
	}

	// Verificar si el archivo ya existe
	existingInodeNum, _ := findFileInDirectoryEXT3MKFILE(file, ext3fs, parentInodeNum, fileName)
	if existingInodeNum != -1 {
		fmt.Printf("El archivo '%s' ya existe. ¿Desea sobreescribirlo? (s/N): ", filePath)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))

		if response != "s" && response != "sí" && response != "si" && response != "yes" && response != "y" {
			return fmt.Errorf("operación cancelada por el usuario")
		}

		// Eliminar el archivo existente
		err = deleteFileFromDirectoryEXT3MKFILE(file, ext3fs, parentInodeNum, fileName, existingInodeNum)
		if err != nil {
			return fmt.Errorf("error eliminando archivo existente: %s", err)
		}
	}

	// Obtener el contenido del archivo
	var fileContent []byte
	var actualSize int32

	if hasContent {
		fileContent, err = ioutil.ReadFile(contentFile)
		if err != nil {
			return fmt.Errorf("error leyendo archivo de contenido '%s': %s", contentFile, err)
		}
		actualSize = int32(len(fileContent))
		fmt.Printf("Contenido cargado desde '%s': %d bytes\n", contentFile, actualSize)
	} else if hasSize {
		fileContent = generateNumberContent(size)
		actualSize = size
		fmt.Printf("Contenido generado: %d bytes (números 0-9)\n", actualSize)
	} else {
		fileContent = []byte{}
		actualSize = 0
		fmt.Printf("Archivo vacío creado\n")
	}

	// Crear el archivo en el sistema EXT3
	err = createFileInEXT3System(file, ext3fs, parentInodeNum, fileName, fileContent, actualSize, sessionInfo, partition)
	if err != nil {
		return fmt.Errorf("error creando archivo en sistema EXT3: %s", err)
	}

	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, filePath, fmt.Sprintf("Archivo creado exitosamente: %s", filePath))

	return nil
}

// generateNumberContent genera contenido con números del 0 al 9 repetidamente
func generateNumberContent(size int32) []byte {
	if size == 0 {
		return []byte{}
	}

	content := make([]byte, size)
	for i := int32(0); i < size; i++ {
		content[i] = byte('0' + (i % 10))
	}
	return content
}

// findOrCreateDirectory busca un directorio y lo crea si no existe (con -r)
func findOrCreateDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirPath string, createParents bool, sessionInfo *session.UserSession) (int32, error) {
	if dirPath == "/" {
		return 0, nil // Inodo raíz
	}

	pathComponents := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentInodeNum := int32(0)

	for i, component := range pathComponents {
		if component == "" {
			continue
		}

		foundInodeNum, err := findFileInDirectory(file, ext2fs, currentInodeNum, component)
		if err != nil {
			return -1, fmt.Errorf("error buscando componente '%s': %s", component, err)
		}

		if foundInodeNum == -1 {
			if !createParents {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("el directorio '%s' no existe (use -r para crear directorios padre)", currentPath)
			}

			newInodeNum, err := createDirectoryInEXT2(file, ext2fs, currentInodeNum, component, sessionInfo)
			if err != nil {
				return -1, fmt.Errorf("error creando directorio '%s': %s", component, err)
			}
			currentInodeNum = newInodeNum
			fmt.Printf("Directorio '%s' creado\n", component)
		} else {
			foundInode := &estructuraArchivoCarpeta.Inodo{}
			inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (foundInodeNum * ext2fs.SuperBlock.S_inode_s))
			err = utilities.ReadStruct(file, inodeOffset, foundInode)
			if err != nil {
				return -1, fmt.Errorf("error leyendo inodo: %s", err)
			}

			if !foundInode.IsDirectory() {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("'%s' no es un directorio", currentPath)
			}

			currentInodeNum = foundInodeNum
		}
	}

	return currentInodeNum, nil
}

func findOrCreateDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirPath string, createParents bool, sessionInfo *session.UserSession) (int32, error) {
	if dirPath == "/" {
		return 0, nil // Inodo raíz
	}

	pathComponents := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentInodeNum := int32(0)

	for i, component := range pathComponents {
		if component == "" {
			continue
		}

		foundInodeNum, err := findFileInDirectoryEXT3MKFILE(file, ext3fs, currentInodeNum, component)
		if err != nil {
			return -1, fmt.Errorf("error buscando componente '%s': %s", component, err)
		}

		if foundInodeNum == -1 {
			if !createParents {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("el directorio '%s' no existe (use -r para crear directorios padre)", currentPath)
			}

			newInodeNum, err := createDirectoryInEXT3MKFILE(file, ext3fs, currentInodeNum, component, sessionInfo)
			if err != nil {
				return -1, fmt.Errorf("error creando directorio '%s': %s", component, err)
			}
			currentInodeNum = newInodeNum
			fmt.Printf("Directorio '%s' creado\n", component)
		} else {
			foundInode := &estructuraArchivoCarpeta.Inodo{}
			inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (foundInodeNum * ext3fs.SuperBlock.S_inode_s))
			err = utilities.ReadStruct(file, inodeOffset, foundInode)
			if err != nil {
				return -1, fmt.Errorf("error leyendo inodo: %s", err)
			}

			if !foundInode.IsDirectory() {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("'%s' no es un directorio", currentPath)
			}

			currentInodeNum = foundInodeNum
		}
	}

	return currentInodeNum, nil
}

func createFileInEXT2System(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, parentInodeNum int32, fileName string, content []byte, size int32, sessionInfo *session.UserSession, partition MountedPartition) error {
	// Buscar un inodo libre
	fileInodeNum, err := findFreeInodeEXT2(file, ext2fs)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %s", err)
	}

	// Calcular bloques necesarios para datos (cada bloque almacena 64 bytes)
	blocksNeeded := int32(0)
	if size > 0 {
		blocksNeeded = (size + 63) / 64 // Redondear hacia arriba
	}

	fmt.Printf("DEBUG: Archivo necesita %d bloques para %d bytes\n", blocksNeeded, size)

	// Crear el inodo del archivo ANTES de asignar bloques
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	fileInode.SetFileValues(
		sessionInfo.UID,
		sessionInfo.GID,
		size,
		1,                // Tipo: archivo
		[3]byte{6, 6, 4}, // Permisos: 664
	)

	// Asignar bloques al archivo usando la estrategia correcta
	err = allocateBlocksToFile(file, ext2fs, fileInode, content, blocksNeeded)
	if err != nil {
		return fmt.Errorf("error asignando bloques: %s", err)
	}

	// Escribir el inodo al disco
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (fileInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilities.WriteStruct(file, inodeOffset, fileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo: %s", err)
	}

	// Marcar el inodo como usado
	err = markInodeAsUsed(file, ext2fs, fileInodeNum)
	if err != nil {
		return fmt.Errorf("error marcando inodo como usado: %s", err)
	}

	// Agregar entrada al directorio padre
	err = addFileEntryToDirectory(file, ext2fs, parentInodeNum, fileName, fileInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio: %s", err)
	}

	// Actualizar contadores del superbloque
	ext2fs.SuperBlock.S_free_inodes_count--
	ext2fs.SuperBlock.S_free_blocks_count -= calculateTotalBlocksUsed(blocksNeeded)

	// Escribir superbloque actualizado
	err = utilities.WriteStruct(file, int64(partition.Start), ext2fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	fmt.Printf("Archivo creado:\n")
	fmt.Printf("   Nombre: %s\n", fileName)
	fmt.Printf("   Tamaño: %d bytes\n", size)
	fmt.Printf("   Bloques de datos: %d\n", blocksNeeded)
	fmt.Printf("   Inodo: %d\n", fileInodeNum)

	return nil
}

func createFileInEXT3System(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInodeNum int32, fileName string, content []byte, size int32, sessionInfo *session.UserSession, partition MountedPartition) error {
	// Buscar un inodo libre
	fileInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no hay inodos libres: %s", err)
	}

	// Calcular bloques necesarios para datos (cada bloque almacena 64 bytes)
	blocksNeeded := int32(0)
	if size > 0 {
		blocksNeeded = (size + 63) / 64 // Redondear hacia arriba
	}

	fmt.Printf("DEBUG: Archivo necesita %d bloques para %d bytes\n", blocksNeeded, size)

	// Crear el inodo del archivo ANTES de asignar bloques
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	fileInode.SetFileValues(
		sessionInfo.UID,
		sessionInfo.GID,
		size,
		1,                // Tipo: archivo
		[3]byte{6, 6, 4}, // Permisos: 664
	)

	// Asignar bloques al archivo usando la estrategia correcta
	err = allocateBlocksToFileEXT3(file, ext3fs, fileInode, content, blocksNeeded)
	if err != nil {
		return fmt.Errorf("error asignando bloques: %s", err)
	}

	// Escribir el inodo al disco
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (fileInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilities.WriteStruct(file, inodeOffset, fileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo: %s", err)
	}

	// Marcar el inodo como usado
	err = markInodeAsUsedEXT3(file, ext3fs, fileInodeNum)
	if err != nil {
		return fmt.Errorf("error marcando inodo como usado: %s", err)
	}

	// Agregar entrada al directorio padre
	err = addFileEntryToDirectoryEXT3(file, ext3fs, parentInodeNum, fileName, fileInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio: %s", err)
	}

	// Actualizar contadores del superbloque
	ext3fs.SuperBlock.S_free_inodes_count--
	ext3fs.SuperBlock.S_free_blocks_count -= calculateTotalBlocksUsed(blocksNeeded)

	// Escribir superbloque actualizado
	err = utilities.WriteStruct(file, int64(partition.Start), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	fmt.Printf("Archivo creado:\n")
	fmt.Printf("   Nombre: %s\n", fileName)
	fmt.Printf("   Tamaño: %d bytes\n", size)
	fmt.Printf("   Bloques de datos: %d\n", blocksNeeded)
	fmt.Printf("   Inodo: %d\n", fileInodeNum)

	return nil
}

func allocateBlocksToFile(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, fileInode *estructuraArchivoCarpeta.Inodo, content []byte, blocksNeeded int32) error {
	if blocksNeeded == 0 {
		return nil // Archivo vacío
	}

	var allocatedBlocks []int32

	// 1. Asignar bloques directos (0-11)
	directBlocks := blocksNeeded
	if directBlocks > 12 {
		directBlocks = 12
	}

	for i := int32(0); i < directBlocks; i++ {
		blockNum, err := findFreeBlock(file, ext2fs)
		if err != nil {
			// Limpiar bloques ya asignados
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("no hay bloques libres para datos directos: %s", err)
		}
		allocatedBlocks = append(allocatedBlocks, blockNum)
		fileInode.SetIblock(int(i), blockNum)

		// Escribir contenido al bloque
		err = writeContentToBlock(file, ext2fs, blockNum, content, i*64, 64)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("error escribiendo contenido al bloque %d: %s", blockNum, err)
		}

		// Marcar bloque como usado
		err = markBlockAsUsed(file, ext2fs, blockNum)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("error marcando bloque como usado: %s", err)
		}
	}

	// 2. Si necesitamos más bloques, usar indirección simple (bloque 12)
	if blocksNeeded > 12 {
		indirectBlocks := blocksNeeded - 12
		if indirectBlocks > 16 {
			indirectBlocks = 16 // Máximo 16 bloques en indirección simple
		}

		// Crear bloque de apuntadores para indirección simple
		pointerBlock := bloques.NewBloqueApuntador()

		for i := int32(0); i < indirectBlocks; i++ {
			blockNum, err := findFreeBlock(file, ext2fs)
			if err != nil {
				freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
				return fmt.Errorf("no hay bloques libres para indirección simple: %s", err)
			}
			allocatedBlocks = append(allocatedBlocks, blockNum)
			pointerBlock.SetPointer(int(i), blockNum)

			// Escribir contenido al bloque
			contentOffset := (12 + i) * 64
			err = writeContentToBlock(file, ext2fs, blockNum, content, contentOffset, 64)
			if err != nil {
				freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
				return fmt.Errorf("error escribiendo contenido al bloque indirecto %d: %s", blockNum, err)
			}

			// Marcar bloque como usado
			err = markBlockAsUsed(file, ext2fs, blockNum)
			if err != nil {
				freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
				return fmt.Errorf("error marcando bloque indirecto como usado: %s", err)
			}
		}

		// Asignar bloque para el apuntador indirecto
		indirectBlockNum, err := findFreeBlock(file, ext2fs)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("no hay bloques libres para bloque de apuntadores: %s", err)
		}
		allocatedBlocks = append(allocatedBlocks, indirectBlockNum)

		// Escribir bloque de apuntadores
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (indirectBlockNum * ext2fs.SuperBlock.S_block_s))
		err = utilities.WriteStruct(file, blockOffset, pointerBlock)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("error escribiendo bloque de apuntadores: %s", err)
		}

		// Marcar bloque de apuntadores como usado
		err = markBlockAsUsed(file, ext2fs, indirectBlockNum)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("error marcando bloque de apuntadores como usado: %s", err)
		}

		fileInode.SetIblock(12, indirectBlockNum)
	}

	// 3. Indirección doble (bloque 13) - si necesitamos más de 28 bloques
	if blocksNeeded > 28 {
		doubleIndirectBlocks := blocksNeeded - 28
		if doubleIndirectBlocks > 256 { // 16 * 16 = 256
			doubleIndirectBlocks = 256
		}

		// Crear bloque de apuntadores de nivel 1 (doble indirección)
		level1Pointer := bloques.NewBloqueApuntador()

		// Calcular cuántos bloques de apuntadores de nivel 2 necesitamos
		level2BlocksNeeded := (doubleIndirectBlocks + 15) / 16

		for l2Index := int32(0); l2Index < level2BlocksNeeded; l2Index++ {
			// Crear bloque de apuntadores de nivel 2
			level2Pointer := bloques.NewBloqueApuntador()

			// Calcular cuántos bloques de datos van en este nivel 2
			remainingBlocks := doubleIndirectBlocks - (l2Index * 16)
			blocksInThisL2 := remainingBlocks
			if blocksInThisL2 > 16 {
				blocksInThisL2 = 16
			}

			// Asignar bloques de datos para este nivel 2
			for dataIndex := int32(0); dataIndex < blocksInThisL2; dataIndex++ {
				blockNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
					return fmt.Errorf("no hay bloques libres para indirección doble: %s", err)
				}
				allocatedBlocks = append(allocatedBlocks, blockNum)
				level2Pointer.SetPointer(int(dataIndex), blockNum)

				// Escribir contenido al bloque
				contentOffset := (28 + l2Index*16 + dataIndex) * 64
				err = writeContentToBlock(file, ext2fs, blockNum, content, contentOffset, 64)
				if err != nil {
					freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
					return fmt.Errorf("error escribiendo contenido al bloque doble indirecto: %s", err)
				}

				err = markBlockAsUsed(file, ext2fs, blockNum)
				if err != nil {
					freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
					return fmt.Errorf("error marcando bloque doble indirecto como usado: %s", err)
				}
			}

			// Asignar y escribir bloque de apuntadores de nivel 2
			level2BlockNum, err := findFreeBlock(file, ext2fs)
			if err != nil {
				freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
				return fmt.Errorf("no hay bloques libres para apuntador nivel 2: %s", err)
			}
			allocatedBlocks = append(allocatedBlocks, level2BlockNum)

			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (level2BlockNum * ext2fs.SuperBlock.S_block_s))
			err = utilities.WriteStruct(file, blockOffset, level2Pointer)
			if err != nil {
				freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
				return fmt.Errorf("error escribiendo bloque apuntador nivel 2: %s", err)
			}

			err = markBlockAsUsed(file, ext2fs, level2BlockNum)
			if err != nil {
				freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
				return fmt.Errorf("error marcando bloque apuntador nivel 2 como usado: %s", err)
			}

			level1Pointer.SetPointer(int(l2Index), level2BlockNum)
		}

		// Asignar y escribir bloque de apuntadores de nivel 1
		level1BlockNum, err := findFreeBlock(file, ext2fs)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("no hay bloques libres para apuntador nivel 1: %s", err)
		}
		allocatedBlocks = append(allocatedBlocks, level1BlockNum)

		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (level1BlockNum * ext2fs.SuperBlock.S_block_s))
		err = utilities.WriteStruct(file, blockOffset, level1Pointer)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("error escribiendo bloque apuntador nivel 1: %s", err)
		}

		err = markBlockAsUsed(file, ext2fs, level1BlockNum)
		if err != nil {
			freeAllocatedBlocks(file, ext2fs, allocatedBlocks)
			return fmt.Errorf("error marcando bloque apuntador nivel 1 como usado: %s", err)
		}

		fileInode.SetIblock(13, level1BlockNum)
	}

	// 4. Indirección triple (bloque 14) - para archivos muy grandes
	if blocksNeeded > 284 { // 12 + 16 + 256 = 284
		tripleIndirectBlocks := blocksNeeded - 284
		if tripleIndirectBlocks > 4096 { // 16 * 16 * 16 = 4096
			tripleIndirectBlocks = 4096
		}

		// Implementación similar pero con 3 niveles de indirección
		// Por simplicidad, aquí muestro solo la estructura básica
		fmt.Printf("WARNING: Indirección triple no completamente implementada en este ejemplo\n")
	}

	return nil
}

func allocateBlocksToFileEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, fileInode *estructuraArchivoCarpeta.Inodo, content []byte, blocksNeeded int32) error {
	if blocksNeeded == 0 {
		return nil // Archivo vacío
	}

	var allocatedBlocks []int32

	// 1. Asignar bloques directos (0-11)
	directBlocks := blocksNeeded
	if directBlocks > 12 {
		directBlocks = 12
	}

	for i := int32(0); i < directBlocks; i++ {
		blockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil {
			// Limpiar bloques ya asignados
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("no hay bloques libres para datos directos: %s", err)
		}
		allocatedBlocks = append(allocatedBlocks, blockNum)
		fileInode.SetIblock(int(i), blockNum)

		// Escribir contenido al bloque
		err = writeContentToBlockEXT3(file, ext3fs, blockNum, content, i*64, 64)
		if err != nil {
			// Limpiar bloques ya asignados
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("error escribiendo contenido en bloque %d: %s", blockNum, err)
		}

		err = markBlockAsUsedEXT3(file, ext3fs, blockNum)
		if err != nil {
			// Limpiar bloques ya asignados
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("error marcando bloque como usado: %s", err)
		}
	}

	if blocksNeeded > 12 {
		indirectBlocks := blocksNeeded - 12
		if indirectBlocks > 16 {
			indirectBlocks = 16 // Máximo 16 bloques en indirección simple
		}

		// Crear bloque de apuntadores para indirección simple
		pointerBlock := bloques.NewBloqueApuntador()

		for i := int32(0); i < indirectBlocks; i++ {
			blockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
				return fmt.Errorf("no hay bloques libres para indirección simple: %s", err)
			}
			allocatedBlocks = append(allocatedBlocks, blockNum)
			pointerBlock.SetPointer(int(i), blockNum)

			// Escribir contenido al bloque
			contentOffset := (12 + i) * 64
			err = writeContentToBlockEXT3(file, ext3fs, blockNum, content, contentOffset, 64)
			if err != nil {
				freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
				return fmt.Errorf("error escribiendo contenido al bloque indirecto %d: %s", blockNum, err)
			}

			// Marcar bloque como usado
			err = markBlockAsUsedEXT3(file, ext3fs, blockNum)
			if err != nil {
				freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
				return fmt.Errorf("error marcando bloque indirecto como usado: %s", err)
			}
		}

		// Asignar bloque para el apuntador indirecto
		indirectBlockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil {
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("no hay bloques libres para el apuntador indirecto: %s", err)
		}
		allocatedBlocks = append(allocatedBlocks, indirectBlockNum)
		blackOffset := int64(ext3fs.SuperBlock.S_block_start + (indirectBlockNum * ext3fs.SuperBlock.S_block_s))
		err = utilities.WriteStruct(file, blackOffset, pointerBlock)
		if err != nil {
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("error escribiendo contenido al bloque indirecto %d: %s", indirectBlockNum, err)
		}

		// Marcar bloque como usado
		err = markBlockAsUsedEXT3(file, ext3fs, indirectBlockNum)
		if err != nil {
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("error marcando bloque indirecto como usado: %s", err)
		}

		fileInode.SetIblock(12, indirectBlockNum)
	}

	if blocksNeeded > 28 {
		doubleIndirectBlocks := blocksNeeded - 28
		if doubleIndirectBlocks > 256 { // 16 * 16 = 256
			doubleIndirectBlocks = 256
		}

		// Crear bloque de apuntadores de nivel 1 (doble indirección)
		level1Pointer := bloques.NewBloqueApuntador()

		// Calcular cuántos bloques de apuntadores de nivel 2 necesitamos
		level2BlocksNeeded := (doubleIndirectBlocks + 15) / 16

		for l2Index := int32(0); l2Index < level2BlocksNeeded; l2Index++ {
			// Crear bloque de apuntadores de nivel 2
			level2Pointer := bloques.NewBloqueApuntador()

			// Calcular cuántos bloques de datos van en este nivel 2
			remainingBlocks := doubleIndirectBlocks - (l2Index * 16)
			blocksInThisL2 := remainingBlocks
			if blocksInThisL2 > 16 {
				blocksInThisL2 = 16
			}

			// Asignar bloques de datos para este nivel 2
			for dataIndex := int32(0); dataIndex < blocksInThisL2; dataIndex++ {
				blockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
					return fmt.Errorf("no hay bloques libres para indirección doble: %s", err)
				}
				allocatedBlocks = append(allocatedBlocks, blockNum)
				level2Pointer.SetPointer(int(dataIndex), blockNum)

				// Escribir contenido al bloque
				contentOffset := (28 + l2Index*16 + dataIndex) * 64
				err = writeContentToBlockEXT3(file, ext3fs, blockNum, content, contentOffset, 64)
				if err != nil {
					freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
					return fmt.Errorf("error escribiendo contenido al bloque doble indirecto: %s", err)
				}

				err = markBlockAsUsedEXT3(file, ext3fs, blockNum)
				if err != nil {
					freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
					return fmt.Errorf("error marcando bloque doble indirecto como usado: %s", err)
				}
			}

			// Asignar y escribir bloque de apuntadores de nivel 2
			level2BlockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
				return fmt.Errorf("no hay bloques libres para apuntador nivel 2: %s", err)
			}
			allocatedBlocks = append(allocatedBlocks, level2BlockNum)

			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (level2BlockNum * ext3fs.SuperBlock.S_block_s))
			err = utilities.WriteStruct(file, blockOffset, level2Pointer)
			if err != nil {
				freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
				return fmt.Errorf("error escribiendo bloque apuntador nivel 2: %s", err)
			}

			err = markBlockAsUsedEXT3(file, ext3fs, level2BlockNum)
			if err != nil {
				freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
				return fmt.Errorf("error marcando bloque apuntador nivel 2 como usado: %s", err)
			}

			level1Pointer.SetPointer(int(l2Index), level2BlockNum)
		}

		// Asignar y escribir bloque de apuntadores de nivel 1
		level1BlockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil {
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("no hay bloques libres para apuntador nivel 1: %s", err)
		}
		allocatedBlocks = append(allocatedBlocks, level1BlockNum)

		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (level1BlockNum * ext3fs.SuperBlock.S_block_s))
		err = utilities.WriteStruct(file, blockOffset, level1Pointer)
		if err != nil {
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("error escribiendo bloque apuntador nivel 1: %s", err)
		}

		err = markBlockAsUsedEXT3(file, ext3fs, level1BlockNum)
		if err != nil {
			freeAllocatedBlocksEXT3(file, ext3fs, allocatedBlocks)
			return fmt.Errorf("error marcando bloque apuntador nivel 1 como usado: %s", err)
		}

		fileInode.SetIblock(13, level1BlockNum)
	}

	// 4. Indirección triple (bloque 14) - para archivos muy grandes
	if blocksNeeded > 284 { // 12 + 16 + 256 = 284
		tripleIndirectBlocks := blocksNeeded - 284
		if tripleIndirectBlocks > 4096 { // 16 * 16 * 16 = 4096
			tripleIndirectBlocks = 4096
		}

		// Implementación similar pero con 3 niveles de indirección
		// Por simplicidad, aquí muestro solo la estructura básica
		fmt.Printf("WARNING: Indirección triple no completamente implementada en este ejemplo\n")
	}

	return nil
}

func writeContentToBlock(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, content []byte, contentOffset int32, maxBytes int32) error {
	fileBlock := bloques.NewBloqueArchivo()
	fileBlock.Clear()

	// Calcular qué porción del contenido escribir
	if contentOffset < int32(len(content)) {
		endOffset := contentOffset + maxBytes
		if endOffset > int32(len(content)) {
			endOffset = int32(len(content))
		}

		bytesToWrite := endOffset - contentOffset
		if bytesToWrite > 0 {
			copy(fileBlock.B_content[:bytesToWrite], content[contentOffset:endOffset])
		}
	}

	// Escribir bloque al disco
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	return utilities.WriteStruct(file, blockOffset, fileBlock)
}

func writeContentToBlockEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32, content []byte, contentOffset int32, maxBytes int32) error {
	fileBlock := bloques.NewBloqueArchivo()
	fileBlock.Clear()

	// Calcular qué porción del contenido escribir
	if contentOffset < int32(len(content)) {
		endOffset := contentOffset + maxBytes
		if endOffset > int32(len(content)) {
			endOffset = int32(len(content))
		}

		bytesToWrite := endOffset - contentOffset
		if bytesToWrite > 0 {
			copy(fileBlock.B_content[:bytesToWrite], content[contentOffset:endOffset])
		}
	}

	// Escribir bloque al disco
	blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
	return utilities.WriteStruct(file, blockOffset, fileBlock)
}

func calculateTotalBlocksUsed(dataBlocks int32) int32 {
	totalBlocks := dataBlocks

	// Agregar bloques de metadatos (apuntadores)
	if dataBlocks > 12 {
		totalBlocks++ // Bloque para indirección simple
		if dataBlocks > 28 {
			// Calcular bloques de apuntadores para indirección doble
			level2Blocks := (dataBlocks - 28 + 15) / 16
			totalBlocks += level2Blocks + 1 // +1 para el bloque nivel 1

			if dataBlocks > 284 {
				// Agregar cálculo para indirección triple si es necesario
				// Esto es más complejo pero sigue el mismo patrón
			}
		}
	}

	return totalBlocks
}

// NUEVA FUNCIÓN: freeAllocatedBlocks libera bloques en caso de error
func freeAllocatedBlocks(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blocks []int32) {
	for _, blockNum := range blocks {
		markBlockAsFree(file, ext2fs, blockNum)
	}
}

func freeAllocatedBlocksEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blocks []int32) {
	for _, blockNum := range blocks {
		markBlockAsFreeEXT3For(file, ext3fs, blockNum)
	}
}

// markInodeAsUsed marca un inodo como usado en el bitmap
func markInodeAsUsed(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32) error {
	return updateInodeBitmap(file, ext2fs, inodeNum, true)
}

// updateInodeBitmap actualiza el bitmap de inodos
func updateInodeBitmap(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32, used bool) error {
	byteIndex := inodeNum / 8
	bitIndex := inodeNum % 8

	// Leer el byte específico del bitmap
	_, err := file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	var currentByte [1]byte
	_, err = file.Read(currentByte[:])
	if err != nil {
		return err
	}

	// Modificar el bit
	mask := byte(1 << (7 - bitIndex))
	if used {
		currentByte[0] |= mask // Poner bit en 1 (usado)
	} else {
		currentByte[0] &^= mask // Poner bit en 0 (libre)
	}

	// Escribir el byte modificado
	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	_, err = file.Write(currentByte[:])
	return err
}

func addFileEntryToDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, fileName string, fileInodeNum int32) error {
	// Leer inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (dirInodeNum * ext2fs.SuperBlock.S_inode_s))
	if err := utilities.ReadStruct(file, inodeOffset, dirInode); err != nil {
		return fmt.Errorf("error leyendo inodo del directorio: %s", err)
	}

	// Revisar bloques directos del directorio
	for i := 0; i < 12; i++ {
		blockNum := dirInode.Iblock[i]
		// Si el bloque no existe, crear uno y añadir la entrada
		if blockNum == -1 {
			newBlock, err := findFreeBlock(file, ext2fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para el directorio: %s", err)
			}
			// Crear bloque carpeta y agregar la entrada
			newDirBlock := bloques.NewBloqueCarpeta()
			newDirBlock.Clear()
			newDirBlock.AddEntry(fileName, fileInodeNum)

			// Escribir bloque en disco
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (newBlock * ext2fs.SuperBlock.S_block_s))
			if err := utilities.WriteStruct(file, blockOffset, newDirBlock); err != nil {
				_ = markBlockAsFree(file, ext2fs, newBlock)
				return fmt.Errorf("error escribiendo nuevo bloque de directorio: %s", err)
			}
			// Marcar bloque usado en bitmap
			if err := markBlockAsUsed(file, ext2fs, newBlock); err != nil {
				_ = markBlockAsFree(file, ext2fs, newBlock)
				return fmt.Errorf("error marcando bloque como usado: %s", err)
			}
			// Actualizar inodo del directorio para apuntar al nuevo bloque
			dirInode.SetIblock(i, newBlock)
			if err := utilities.WriteStruct(file, inodeOffset, dirInode); err != nil {
				return fmt.Errorf("error actualizando inodo del directorio: %s", err)
			}
			return nil
		}

		// Si el bloque existe, leerlo y tratar de insertar la entrada
		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		if err := utilities.ReadStruct(file, blockOffset, dirBlock); err != nil {
			return fmt.Errorf("error leyendo bloque de directorio %d: %s", blockNum, err)
		}

		// Si hay espacio, añadir y escribir
		if !dirBlock.IsFull() {
			if !dirBlock.AddEntry(fileName, fileInodeNum) {
				return fmt.Errorf("error añadiendo entrada al bloque %d", blockNum)
			}
			if err := utilities.WriteStruct(file, blockOffset, dirBlock); err != nil {
				return fmt.Errorf("error escribiendo bloque de directorio %d: %s", blockNum, err)
			}
			return nil
		}
	}

	// Si llegamos aquí, todos los directos están llenos => tratar indirectos (simple/doble/triple)
	for i := 12; i < 15; i++ {
		switch i {
		case 12: // indirecto simple: inodo.Iblock[12] -> BloqueApuntador -> Bloques de carpeta
			ptrBlockNum := dirInode.Iblock[12]
			// crear bloque apuntador + primer bloque de carpeta si necesario
			if ptrBlockNum == -1 {
				// crear bloque de datos (carpeta)
				newDirBlockNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para directorio (indirecto simple): %s", err)
				}
				newDirBlock := bloques.NewBloqueCarpeta()
				newDirBlock.Clear()
				if !newDirBlock.AddEntry(fileName, fileInodeNum) {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto simple)")
				}
				newDirOffset := int64(ext2fs.SuperBlock.S_block_start + (newDirBlockNum * ext2fs.SuperBlock.S_block_s))
				if err := utilities.WriteStruct(file, newDirOffset, newDirBlock); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto simple): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
				}

				// crear bloque apuntador y enlazar
				ptrBlock := bloques.NewBloqueApuntador()
				ptrBlock.Clear()
				ptrBlock.B_pointers[0] = newDirBlockNum
				ptrBlockNum, err = findFreeBlock(file, ext2fs)
				if err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("no hay bloques libres para bloque de apuntadores (indirecto simple): %s", err)
				}
				ptrOffset := int64(ext2fs.SuperBlock.S_block_start + (ptrBlockNum * ext2fs.SuperBlock.S_block_s))
				if err := utilities.WriteStruct(file, ptrOffset, ptrBlock); err != nil {
					_ = markBlockAsFree(file, ext2fs, ptrBlockNum)
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo bloque de apuntadores (indirecto simple): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, ptrBlockNum); err != nil {
					return fmt.Errorf("error marcando bloque de apuntadores usado: %s", err)
				}

				// actualizar inodo
				dirInode.SetIblock(12, ptrBlockNum)
				if err := utilities.WriteStruct(file, inodeOffset, dirInode); err != nil {
					return fmt.Errorf("error actualizando inodo del directorio (agregando indirecto simple): %s", err)
				}
				return nil
			}

			// si existe bloque de apuntadores, leer y buscar espacio
			ptrBlock := &bloques.BloqueApuntador{}
			ptrOffset := int64(ext2fs.SuperBlock.S_block_start + (ptrBlockNum * ext2fs.SuperBlock.S_block_s))
			if err := utilities.ReadStruct(file, ptrOffset, ptrBlock); err != nil {
				return fmt.Errorf("error leyendo bloque de apuntadores (indirecto simple): %s", err)
			}

			// recorrer punteros de primer nivel
			for pIdx := 0; pIdx < 16; pIdx++ {
				p := ptrBlock.B_pointers[pIdx]
				if p == -1 {
					// crear nuevo bloque de carpeta y enlazar
					newDirBlockNum, err := findFreeBlock(file, ext2fs)
					if err != nil {
						return fmt.Errorf("no hay bloques libres para agregar bloque de directorio (indirecto simple): %s", err)
					}
					newDirBlock := bloques.NewBloqueCarpeta()
					newDirBlock.Clear()
					if !newDirBlock.AddEntry(fileName, fileInodeNum) {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto simple)")
					}
					newDirOffset := int64(ext2fs.SuperBlock.S_block_start + (newDirBlockNum * ext2fs.SuperBlock.S_block_s))
					if err := utilities.WriteStruct(file, newDirOffset, newDirBlock); err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto simple): %s", err)
					}
					if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
						return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
					}

					ptrBlock.B_pointers[pIdx] = newDirBlockNum
					if err := utilities.WriteStruct(file, ptrOffset, ptrBlock); err != nil {
						return fmt.Errorf("error actualizando bloque de apuntadores (indirecto simple): %s", err)
					}
					return nil
				}

				// si existe, leer ese bloque de carpeta y tratar de insertar
				dirBlock := &bloques.BloqueCarpeta{}
				dirBlockOffset := int64(ext2fs.SuperBlock.S_block_start + (p * ext2fs.SuperBlock.S_block_s))
				if err := utilities.ReadStruct(file, dirBlockOffset, dirBlock); err != nil {
					return fmt.Errorf("error leyendo bloque de carpeta apuntado por indirecto: %s", err)
				}
				if !dirBlock.IsFull() {
					if !dirBlock.AddEntry(fileName, fileInodeNum) {
						return fmt.Errorf("error añadiendo entrada al bloque %d (indirecto simple)", p)
					}
					if err := utilities.WriteStruct(file, dirBlockOffset, dirBlock); err != nil {
						return fmt.Errorf("error escribiendo bloque de directorio %d (indirecto simple): %s", p, err)
					}
					return nil
				}
			}
			// si se llena el bloque de apuntadores, seguir a siguiente tipo
		case 13: // indirecto doble: inodo.Iblock[13] -> nivel1 apuntadores -> nivel2 apuntadores -> bloques carpeta
			ptrLevel1Num := dirInode.Iblock[13]
			// crear nivel1 + nivel2 + bloque carpeta si necesario
			if ptrLevel1Num == -1 {
				// crear nuevo bloque de carpeta
				newDirBlockNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para directorio (indirecto doble): %s", err)
				}
				newDirBlock := bloques.NewBloqueCarpeta()
				newDirBlock.Clear()
				if !newDirBlock.AddEntry(fileName, fileInodeNum) {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto doble)")
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto doble): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
					return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
				}

				// crear nivel2 (apuntadores) que apunte al bloque de carpeta
				level2 := bloques.NewBloqueApuntador()
				level2.Clear()
				level2.B_pointers[0] = newDirBlockNum
				level2Num, err := findFreeBlock(file, ext2fs)
				if err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("no hay bloques libres para nivel2 apuntadores (indirecto doble): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(level2Num*ext2fs.SuperBlock.S_block_s)), level2); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, level2Num)
					return fmt.Errorf("error escribiendo nivel2 apuntadores (indirecto doble): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, level2Num); err != nil {
					return fmt.Errorf("error marcando nivel2 apuntadores usados: %s", err)
				}

				// crear nivel1 que apunte al nivel2
				level1 := bloques.NewBloqueApuntador()
				level1.Clear()
				level1.B_pointers[0] = level2Num
				level1Num, err := findFreeBlock(file, ext2fs)
				if err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, level2Num)
					return fmt.Errorf("no hay bloques libres para nivel1 apuntadores (indirecto doble): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(level1Num*ext2fs.SuperBlock.S_block_s)), level1); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, level2Num)
					_ = markBlockAsFree(file, ext2fs, level1Num)
					return fmt.Errorf("error escribiendo nivel1 apuntadores (indirecto doble): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, level1Num); err != nil {
					return fmt.Errorf("error marcando nivel1 apuntadores usados: %s", err)
				}

				// actualizar inodo
				dirInode.SetIblock(13, level1Num)
				if err := utilities.WriteStruct(file, inodeOffset, dirInode); err != nil {
					return fmt.Errorf("error actualizando inodo del directorio (agregando indirecto doble): %s", err)
				}
				return nil
			}

			// si existe nivel1, leerlo y recorrer sus punteros
			level1 := &bloques.BloqueApuntador{}
			level1Offset := int64(ext2fs.SuperBlock.S_block_start + (ptrLevel1Num * ext2fs.SuperBlock.S_block_s))
			if err := utilities.ReadStruct(file, level1Offset, level1); err != nil {
				return fmt.Errorf("error leyendo nivel1 apuntadores (indirecto doble): %s", err)
			}
			for idx1 := 0; idx1 < 16; idx1++ {
				level2Num := level1.B_pointers[idx1]
				if level2Num == -1 {
					// crear level2 + nuevo bloque carpeta
					newDirBlockNum, err := findFreeBlock(file, ext2fs)
					if err != nil {
						return fmt.Errorf("no hay bloques libres para directorio (indirecto doble, crear level2): %s", err)
					}
					newDirBlock := bloques.NewBloqueCarpeta()
					newDirBlock.Clear()
					if !newDirBlock.AddEntry(fileName, fileInodeNum) {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto doble)")
					}
					if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto doble): %s", err)
					}
					if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
						return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
					}

					level2 := bloques.NewBloqueApuntador()
					level2.Clear()
					level2.B_pointers[0] = newDirBlockNum
					level2Num, err := findFreeBlock(file, ext2fs)
					if err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("no hay bloques libres para level2 apuntadores (indirecto doble): %s", err)
					}
					if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(level2Num*ext2fs.SuperBlock.S_block_s)), level2); err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						_ = markBlockAsFree(file, ext2fs, level2Num)
						return fmt.Errorf("error escribiendo level2 apuntadores (indirecto doble): %s", err)
					}
					if err := markBlockAsUsed(file, ext2fs, level2Num); err != nil {
						return fmt.Errorf("error marcando level2 apuntadores usados: %s", err)
					}

					// enlazar level2 en level1 y escribir
					level1.B_pointers[idx1] = level2Num
					if err := utilities.WriteStruct(file, level1Offset, level1); err != nil {
						return fmt.Errorf("error actualizando level1 apuntadores (indirecto doble): %s", err)
					}
					return nil
				}

				// level2 existe -> leerlo y recorrer sus punteros a bloques carpeta
				level2 := &bloques.BloqueApuntador{}
				level2Offset := int64(ext2fs.SuperBlock.S_block_start + (level2Num * ext2fs.SuperBlock.S_block_s))
				if err := utilities.ReadStruct(file, level2Offset, level2); err != nil {
					return fmt.Errorf("error leyendo level2 apuntadores (indirecto doble): %s", err)
				}
				for idx2 := 0; idx2 < 16; idx2++ {
					p := level2.B_pointers[idx2]
					if p == -1 {
						// crear nuevo bloque carpeta y enlazar
						newDirBlockNum, err := findFreeBlock(file, ext2fs)
						if err != nil {
							return fmt.Errorf("no hay bloques libres para agregar bloque de directorio (indirecto doble): %s", err)
						}
						newDirBlock := bloques.NewBloqueCarpeta()
						newDirBlock.Clear()
						if !newDirBlock.AddEntry(fileName, fileInodeNum) {
							_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
							return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto doble)")
						}
						if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
							_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
							return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto doble): %s", err)
						}
						if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
							return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
						}

						level2.B_pointers[idx2] = newDirBlockNum
						if err := utilities.WriteStruct(file, level2Offset, level2); err != nil {
							return fmt.Errorf("error actualizando level2 apuntadores (indirecto doble): %s", err)
						}
						return nil
					}

					// si p existe, leer bloque carpeta y tratar de insertar
					dirBlock := &bloques.BloqueCarpeta{}
					dirBlockOffset := int64(ext2fs.SuperBlock.S_block_start + (p * ext2fs.SuperBlock.S_block_s))
					if err := utilities.ReadStruct(file, dirBlockOffset, dirBlock); err != nil {
						return fmt.Errorf("error leyendo bloque de carpeta apuntado por indirecto doble: %s", err)
					}
					if !dirBlock.IsFull() {
						if !dirBlock.AddEntry(fileName, fileInodeNum) {
							return fmt.Errorf("error añadiendo entrada al bloque %d (indirecto doble)", p)
						}
						if err := utilities.WriteStruct(file, dirBlockOffset, dirBlock); err != nil {
							return fmt.Errorf("error escribiendo bloque de directorio %d (indirecto doble): %s", p, err)
						}
						return nil
					}
				}
			}
		case 14: // indirecto triple: nivel1 -> nivel2 -> nivel3 -> bloques carpeta
			ptrL1 := dirInode.Iblock[14]
			if ptrL1 == -1 {
				// crear nuevo bloque carpeta
				newDirBlockNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para directorio (indirecto triple): %s", err)
				}
				newDirBlock := bloques.NewBloqueCarpeta()
				newDirBlock.Clear()
				if !newDirBlock.AddEntry(fileName, fileInodeNum) {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
					return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
				}

				// crear nivel3 que apunte al bloque carpeta
				l3 := bloques.NewBloqueApuntador()
				l3.Clear()
				l3.B_pointers[0] = newDirBlockNum
				l3Num, err := findFreeBlock(file, ext2fs)
				if err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					return fmt.Errorf("no hay bloques libres para nivel3 apuntadores (indirecto triple): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(l3Num*ext2fs.SuperBlock.S_block_s)), l3); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, l3Num)
					return fmt.Errorf("error escribiendo nivel3 apuntadores (indirecto triple): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, l3Num); err != nil {
					return fmt.Errorf("error marcando nivel3 apuntadores usados: %s", err)
				}

				// crear nivel2 que apunte al nivel3
				l2 := bloques.NewBloqueApuntador()
				l2.Clear()
				l2.B_pointers[0] = l3Num
				l2Num, err := findFreeBlock(file, ext2fs)
				if err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, l3Num)
					return fmt.Errorf("no hay bloques libres para nivel2 apuntadores (indirecto triple): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(l2Num*ext2fs.SuperBlock.S_block_s)), l2); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, l3Num)
					_ = markBlockAsFree(file, ext2fs, l2Num)
					return fmt.Errorf("error escribiendo nivel2 apuntadores (indirecto triple): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, l2Num); err != nil {
					return fmt.Errorf("error marcando nivel2 apuntadores usados: %s", err)
				}

				// crear nivel1 que apunte al nivel2
				l1 := bloques.NewBloqueApuntador()
				l1.Clear()
				l1.B_pointers[0] = l2Num
				l1Num, err := findFreeBlock(file, ext2fs)
				if err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, l3Num)
					_ = markBlockAsFree(file, ext2fs, l2Num)
					return fmt.Errorf("no hay bloques libres para nivel1 apuntadores (indirecto triple): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(l1Num*ext2fs.SuperBlock.S_block_s)), l1); err != nil {
					_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
					_ = markBlockAsFree(file, ext2fs, l3Num)
					_ = markBlockAsFree(file, ext2fs, l2Num)
					_ = markBlockAsFree(file, ext2fs, l1Num)
					return fmt.Errorf("error escribiendo nivel1 apuntadores (indirecto triple): %s", err)
				}
				if err := markBlockAsUsed(file, ext2fs, l1Num); err != nil {
					return fmt.Errorf("error marcando nivel1 apuntadores usados: %s", err)
				}

				// actualizar inodo
				dirInode.SetIblock(14, l1Num)
				if err := utilities.WriteStruct(file, inodeOffset, dirInode); err != nil {
					return fmt.Errorf("error actualizando inodo del directorio (agregando indirecto triple): %s", err)
				}
				return nil
			}

			// si existe l1, leer y recorrer: l1 -> l2 -> l3 -> bloques carpeta
			l1 := &bloques.BloqueApuntador{}
			l1Offset := int64(ext2fs.SuperBlock.S_block_start + (ptrL1 * ext2fs.SuperBlock.S_block_s))
			if err := utilities.ReadStruct(file, l1Offset, l1); err != nil {
				return fmt.Errorf("error leyendo nivel1 apuntadores (indirecto triple): %s", err)
			}
			for idx1 := 0; idx1 < 16; idx1++ {
				l2Num := l1.B_pointers[idx1]
				if l2Num == -1 {
					// crear l2 + l3 + bloque carpeta
					newDirBlockNum, err := findFreeBlock(file, ext2fs)
					if err != nil {
						return fmt.Errorf("no hay bloques libres para directorio (indirecto triple, crear l2): %s", err)
					}
					newDirBlock := bloques.NewBloqueCarpeta()
					newDirBlock.Clear()
					if !newDirBlock.AddEntry(fileName, fileInodeNum) {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
					}
					if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
					}
					if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
						return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
					}

					l3 := bloques.NewBloqueApuntador()
					l3.Clear()
					l3.B_pointers[0] = newDirBlockNum
					l3Num, err := findFreeBlock(file, ext2fs)
					if err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						return fmt.Errorf("no hay bloques libres para l3 apuntadores (indirecto triple): %s", err)
					}
					if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(l3Num*ext2fs.SuperBlock.S_block_s)), l3); err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						_ = markBlockAsFree(file, ext2fs, l3Num)
						return fmt.Errorf("error escribiendo l3 apuntadores (indirecto triple): %s", err)
					}
					if err := markBlockAsUsed(file, ext2fs, l3Num); err != nil {
						return fmt.Errorf("error marcando l3 apuntadores usados: %s", err)
					}

					l2 := bloques.NewBloqueApuntador()
					l2.Clear()
					l2.B_pointers[0] = l3Num
					l2Num, err := findFreeBlock(file, ext2fs)
					if err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						_ = markBlockAsFree(file, ext2fs, l3Num)
						return fmt.Errorf("no hay bloques libres para l2 apuntadores (indirecto triple): %s", err)
					}
					if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(l2Num*ext2fs.SuperBlock.S_block_s)), l2); err != nil {
						_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
						_ = markBlockAsFree(file, ext2fs, l3Num)
						_ = markBlockAsFree(file, ext2fs, l2Num)
						return fmt.Errorf("error escribiendo l2 apuntadores (indirecto triple): %s", err)
					}
					if err := markBlockAsUsed(file, ext2fs, l2Num); err != nil {
						return fmt.Errorf("error marcando l2 apuntadores usados: %s", err)
					}

					l1.B_pointers[idx1] = l2Num
					if err := utilities.WriteStruct(file, l1Offset, l1); err != nil {
						return fmt.Errorf("error actualizando l1 apuntadores (indirecto triple): %s", err)
					}
					return nil
				}

				// l2 existe: leer y recorrer
				l2 := &bloques.BloqueApuntador{}
				l2Offset := int64(ext2fs.SuperBlock.S_block_start + (l2Num * ext2fs.SuperBlock.S_block_s))
				if err := utilities.ReadStruct(file, l2Offset, l2); err != nil {
					return fmt.Errorf("error leyendo l2 apuntadores (indirecto triple): %s", err)
				}
				for idx2 := 0; idx2 < 16; idx2++ {
					l3Num := l2.B_pointers[idx2]
					if l3Num == -1 {
						// crear l3 + nuevo bloque carpeta
						newDirBlockNum, err := findFreeBlock(file, ext2fs)
						if err != nil {
							return fmt.Errorf("no hay bloques libres para directorio (indirecto triple, crear l3): %s", err)
						}
						newDirBlock := bloques.NewBloqueCarpeta()
						newDirBlock.Clear()
						if !newDirBlock.AddEntry(fileName, fileInodeNum) {
							_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
							return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
						}
						if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
							_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
							return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
						}
						if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
							return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
						}

						l3 := bloques.NewBloqueApuntador()
						l3.Clear()
						l3.B_pointers[0] = newDirBlockNum
						l3NumNew, err := findFreeBlock(file, ext2fs)
						if err != nil {
							_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
							return fmt.Errorf("no hay bloques libres para l3 apuntadores (indirecto triple): %s", err)
						}
						if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(l3NumNew*ext2fs.SuperBlock.S_block_s)), l3); err != nil {
							_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
							_ = markBlockAsFree(file, ext2fs, l3NumNew)
							return fmt.Errorf("error escribiendo l3 apuntadores (indirecto triple): %s", err)
						}
						if err := markBlockAsUsed(file, ext2fs, l3NumNew); err != nil {
							return fmt.Errorf("error marcando l3 apuntadores usados: %s", err)
						}

						l2.B_pointers[idx2] = l3NumNew
						if err := utilities.WriteStruct(file, l2Offset, l2); err != nil {
							return fmt.Errorf("error actualizando l2 apuntadores (indirecto triple): %s", err)
						}
						return nil
					}

					// l3 existe: leer y recorrer bloques carpeta apuntados
					l3 := &bloques.BloqueApuntador{}
					l3Offset := int64(ext2fs.SuperBlock.S_block_start + (l3Num * ext2fs.SuperBlock.S_block_s))
					if err := utilities.ReadStruct(file, l3Offset, l3); err != nil {
						return fmt.Errorf("error leyendo l3 apuntadores (indirecto triple): %s", err)
					}
					for idx3 := 0; idx3 < 16; idx3++ {
						p := l3.B_pointers[idx3]
						if p == -1 {
							// crear bloque carpeta y enlazar
							newDirBlockNum, err := findFreeBlock(file, ext2fs)
							if err != nil {
								return fmt.Errorf("no hay bloques libres para agregar bloque de directorio (indirecto triple): %s", err)
							}
							newDirBlock := bloques.NewBloqueCarpeta()
							newDirBlock.Clear()
							if !newDirBlock.AddEntry(fileName, fileInodeNum) {
								_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
								return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
							}
							if err := utilities.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+(newDirBlockNum*ext2fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
								_ = markBlockAsFree(file, ext2fs, newDirBlockNum)
								return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
							}
							if err := markBlockAsUsed(file, ext2fs, newDirBlockNum); err != nil {
								return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
							}

							l3.B_pointers[idx3] = newDirBlockNum
							if err := utilities.WriteStruct(file, l3Offset, l3); err != nil {
								return fmt.Errorf("error actualizando l3 apuntadores (indirecto triple): %s", err)
							}
							return nil
						}

						// si p existe, leer bloque carpeta y tratar de insertar
						dirBlock := &bloques.BloqueCarpeta{}
						dirBlockOffset := int64(ext2fs.SuperBlock.S_block_start + (p * ext2fs.SuperBlock.S_block_s))
						if err := utilities.ReadStruct(file, dirBlockOffset, dirBlock); err != nil {
							return fmt.Errorf("error leyendo bloque de carpeta apuntado por indirecto triple: %s", err)
						}
						if !dirBlock.IsFull() {
							if !dirBlock.AddEntry(fileName, fileInodeNum) {
								return fmt.Errorf("error añadiendo entrada al bloque %d (indirecto triple)", p)
							}
							if err := utilities.WriteStruct(file, dirBlockOffset, dirBlock); err != nil {
								return fmt.Errorf("error escribiendo bloque de directorio %d (indirecto triple): %s", p, err)
							}
							return nil
						}
					}
				}
			}
		}
	}

	return fmt.Errorf("no hay espacio en el directorio para agregar la entrada")
}

// findFileInDirectory busca un archivo en un directorio específico
func findFileInDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, fileName string) (int32, error) {
	// Leer el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (dirInodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilities.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return -1, err
	}

	// Verificar que es un directorio
	if !dirInode.IsDirectory() {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en los bloques del directorio
	blocks := dirInode.GetIblock()

	for i := 0; i < 12 && blocks[i] != -1; i++ {
		blockNum := blocks[i]

		// Leer el bloque de carpeta
		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err = utilities.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			return -1, err
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo != -1 {
				// Convertir el nombre del archivo a string
				entryName := string(dirBlock.B_content[j].B_name[:])
				entryName = strings.TrimRight(entryName, "\x00") // Quitar caracteres nulos

				if entryName == fileName {
					return dirBlock.B_content[j].B_inodo, nil
				}
			}
		}
	}

	return -1, nil // No encontrado
}

func findFileInDirectoryEXT3MKFILE(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInodeNum int32, fileName string) (int32, error) {
	// Leer el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (dirInodeNum * ext3fs.SuperBlock.S_inode_s))
	err := utilities.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return -1, err
	}

	// Verificar que es un directorio
	if !dirInode.IsDirectory() {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en los bloques del directorio
	blocks := dirInode.GetIblock()

	for i := 0; i < 12 && blocks[i] != -1; i++ {
		blockNum := blocks[i]

		// Leer el bloque de carpeta
		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err = utilities.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			return -1, err
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo != -1 {
				// Convertir el nombre del archivo a string
				entryName := string(dirBlock.B_content[j].B_name[:])
				entryName = strings.TrimRight(entryName, "\x00") // Quitar caracteres nulos

				if entryName == fileName {
					return dirBlock.B_content[j].B_inodo, nil
				}
			}
		}
	}

	return -1, nil // No encontrado
}

// deleteFileFromDirectory elimina un archivo de un directorio
func deleteFileFromDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, fileName string, fileInodeNum int32) error {
	// Leer el inodo del archivo a eliminar
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (fileInodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilities.ReadStruct(file, inodeOffset, fileInode)
	if err != nil {
		return err
	}

	// Liberar bloques del archivo
	blocks := fileInode.GetIblock()
	blocksFreed := int32(0)

	for i := 0; i < 12 && blocks[i] != -1; i++ {
		blockNum := blocks[i]

		// Limpiar el bloque
		emptyBlock := &bloques.BloqueArchivo{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err = utilities.WriteStruct(file, blockOffset, emptyBlock)
		if err != nil {
			return err
		}

		// Marcar bloque como libre en bitmap
		err = markBlockAsFree(file, ext2fs, blockNum)
		if err != nil {
			return err
		}

		blocksFreed++
	}

	// Limpiar el inodo
	emptyInode := &estructuraArchivoCarpeta.Inodo{}
	err = utilities.WriteStruct(file, inodeOffset, emptyInode)
	if err != nil {
		return err
	}

	// Marcar inodo como libre en bitmap
	err = markInodeAsFree(file, ext2fs, fileInodeNum)
	if err != nil {
		return err
	}

	// Remover entrada del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (dirInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilities.ReadStruct(file, dirInodeOffset, dirInode)
	if err != nil {
		return err
	}

	dirBlocks := dirInode.GetIblock()

	for i := 0; i < 12 && dirBlocks[i] != -1; i++ {
		blockNum := dirBlocks[i]

		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err = utilities.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			return err
		}

		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo == fileInodeNum {
				// Entrada encontrada, limpiarla
				dirBlock.B_content[j].B_inodo = -1
				for k := 0; k < len(dirBlock.B_content[j].B_name); k++ {
					dirBlock.B_content[j].B_name[k] = 0
				}

				// Escribir el bloque actualizado
				err = utilities.WriteStruct(file, blockOffset, dirBlock)
				if err != nil {
					return err
				}

				// Actualizar contadores del superbloque
				ext2fs.SuperBlock.S_free_inodes_count++
				ext2fs.SuperBlock.S_free_blocks_count += blocksFreed

				return nil
			}
		}
	}

	return fmt.Errorf("entrada de archivo no encontrada en el directorio")
}

func deleteFileFromDirectoryEXT3MKFILE(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInodeNum int32, fileName string, fileInodeNum int32) error {
	// Leer el inodo del archivo a eliminar
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (fileInodeNum * ext3fs.SuperBlock.S_inode_s))
	err := utilities.ReadStruct(file, inodeOffset, fileInode)
	if err != nil {
		return err
	}

	// Liberar bloques del archivo
	blocks := fileInode.GetIblock()
	blocksFreed := int32(0)

	for i := 0; i < 12 && blocks[i] != -1; i++ {
		blockNum := blocks[i]

		// Limpiar el bloque
		emptyBlock := &bloques.BloqueArchivo{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err = utilities.WriteStruct(file, blockOffset, emptyBlock)
		if err != nil {
			return err
		}

		err = markBlockAsFreeEXT3For(file, ext3fs, blockNum)
		if err != nil {
			return err
		}

		blocksFreed++

	}

	// Limpiar el inodo
	emptyInode := &estructuraArchivoCarpeta.Inodo{}
	err = utilities.WriteStruct(file, inodeOffset, emptyInode)
	if err != nil {
		return err
	}

	err = MarkInodeAsFreeEXT3(file, ext3fs, fileInodeNum)
	if err != nil {
		return err
	}

	// Remover entrada del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (dirInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilities.ReadStruct(file, dirInodeOffset, dirInode)
	if err != nil {
		return err
	}

	dirBlocks := dirInode.GetIblock()

	for i := 0; i < 12 && dirBlocks[i] != -1; i++ {
		blockNum := dirBlocks[i]

		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err = utilities.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			return err
		}

		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo == fileInodeNum {
				// Entrada encontrada, limpiarla
				dirBlock.B_content[j].B_inodo = -1
				for k := 0; k < len(dirBlock.B_content[j].B_name); k++ {
					dirBlock.B_content[j].B_name[k] = 0
				}

				// Escribir el bloque actualizado
				err = utilities.WriteStruct(file, blockOffset, dirBlock)
				if err != nil {
					return err
				}

				// Actualizar contadores del superbloque
				ext3fs.SuperBlock.S_free_inodes_count++
				ext3fs.SuperBlock.S_free_blocks_count += blocksFreed

				return nil
			}
		}
	}

	return fmt.Errorf("entrada de archivo no encontrada en el directorio")
}

// createDirectoryInEXT2 crea un nuevo directorio en el sistema EXT2
func createDirectoryInEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := findFreeInodeEXT2(file, ext2fs)
	if err != nil {
		return -1, fmt.Errorf("no hay inodos libres: %s", err)
	}

	// Buscar un bloque libre para el directorio
	blockNum, err := findFreeBlock(file, ext2fs)
	if err != nil {
		return -1, fmt.Errorf("no hay bloques libres: %s", err)
	}

	// Crear el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInode.SetFileValues(
		sessionInfo.UID,  // UID del usuario actual
		sessionInfo.GID,  // GID del usuario actual
		64,               // Tamaño típico de directorio
		0,                // Tipo: directorio
		[3]byte{7, 5, 5}, // Permisos: 755 (rwxr-xr-x)
	)

	// Asignar el bloque al directorio
	dirInode.SetIblock(0, blockNum)

	// Crear el bloque del directorio con entradas . y ..
	dirBlock := &bloques.BloqueCarpeta{}

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
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	err = utilities.WriteStruct(file, blockOffset, dirBlock)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo bloque de directorio: %s", err)
	}

	// Escribir el inodo del directorio
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (newInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilities.WriteStruct(file, inodeOffset, dirInode)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo inodo de directorio: %s", err)
	}

	// Marcar inodo y bloque como usados
	err = markInodeAsUsed(file, ext2fs, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error marcando inodo como usado: %s", err)
	}

	err = markBlockAsUsed(file, ext2fs, blockNum)
	if err != nil {
		return -1, fmt.Errorf("error marcando bloque como usado: %s", err)
	}

	// Agregar entrada al directorio padre
	err = addFileEntryToDirectory(file, ext2fs, parentInodeNum, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %s", err)
	}

	// Actualizar contadores del superbloque
	ext2fs.SuperBlock.S_free_inodes_count--
	ext2fs.SuperBlock.S_free_blocks_count--

	return newInodeNum, nil
}

func createDirectoryInEXT3MKFILE(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return -1, fmt.Errorf("no hay inodos libres: %s", err)
	}

	// Buscar un bloque libre para el directorio
	blockNum, err := findFreeBlockEXT3(file, ext3fs)
	if err != nil {
		return -1, fmt.Errorf("no hay bloques libres: %s", err)
	}

	// Crear el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInode.SetFileValues(
		sessionInfo.UID,  // UID del usuario actual
		sessionInfo.GID,  // GID del usuario actual
		64,               // Tamaño típico de directorio
		0,                // Tipo: directorio
		[3]byte{7, 5, 5}, // Permisos: 755 (rwxr-xr-x)
	)

	// Asignar el bloque al directorio
	dirInode.SetIblock(0, blockNum)

	// Crear el bloque del directorio con entradas . y ..
	dirBlock := &bloques.BloqueCarpeta{}

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
	err = utilities.WriteStruct(file, blockOffset, dirBlock)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo bloque de directorio: %s", err)
	}

	// Escribir el inodo del directorio
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (newInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilities.WriteStruct(file, inodeOffset, dirInode)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo inodo de directorio: %s", err)
	}

	// Marcar inodo y bloque como usados
	err = markInodeAsUsedEXT3(file, ext3fs, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error marcando inodo como usado: %s", err)
	}

	err = markBlockAsUsedEXT3For(file, ext3fs, blockNum)
	if err != nil {
		return -1, fmt.Errorf("error marcando bloque como usado: %s", err)
	}

	// Agregar entrada al directorio padre
	err = addFileEntryToDirectoryEXT3(file, ext3fs, parentInodeNum, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %s", err)
	}

	// Actualizar contadores del superbloque
	ext3fs.SuperBlock.S_free_inodes_count--
	ext3fs.SuperBlock.S_free_blocks_count--

	return newInodeNum, nil
}

// markInodeAsFree marca un inodo como libre en el bitmap
func markInodeAsFree(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32) error {
	return updateInodeBitmap(file, ext2fs, inodeNum, false)
}

package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilities "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MKDIR implementa el comando para crear directorios
func MKDIR(args []string) {
	// Verificar que hay sesión activa
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		return
	}

	// PARSEAR PARÁMETROS
	params := utilities.ParseParameters(args)

	// EXTRAER Y VALIDAR PARÁMETROS
	dirPath := strings.Trim(params["path"], "\"")
	createParents := utilities.HasParam(params, "p")

	// Validar parámetros obligatorios
	if dirPath == "" {
		fmt.Println("Error: Debe especificar la ruta del directorio a crear")
		fmt.Println("Uso: mkdir -path=<ruta_directorio> [-p]")
		return
	}

	// VALIDACIÓN: -p no debe tener valor
	if pValue, exists := params["p"]; exists && pValue != "" {
		fmt.Println("Error: El parámetro -p no debe recibir ningún valor")
		return
	}

	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(dirPath, "/") {
		fmt.Println("Error: La ruta debe ser absoluta (comenzar con /)")
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

	fsType, err := getFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error obteniendo tipo de sistema de archivos: %s\n", err)
		return
	}

	if fsType == estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		err = createDirectory(mountedPartition, dirPath, createParents, sessionInfo)
	} else if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		err = createDirectoryEXT3(mountedPartition, dirPath, createParents, sessionInfo)
	} else {
		fmt.Printf("Error: El sistema de archivos '%s' no es compatible con mkdir\n", fsType)
		return
	}

	if err != nil {
		fmt.Printf("Error creando directorio: %s\n", err)
		return
	}

	fmt.Printf("Directorio '%s' creado exitosamente\n", dirPath)
}

// createDirectory crea un directorio en el sistema EXT2
func createDirectory(partition MountedPartition, dirPath string, createParents bool, sessionInfo *session.UserSession) error {
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

	// Separar directorio padre y nombre del directorio a crear
	parentDir, dirName := filepath.Split(dirPath)
	parentDir = strings.TrimSuffix(parentDir, "/") // Remover / final
	if parentDir == "" {
		parentDir = "/"
	}

	fmt.Printf("DEBUG: Creando directorio '%s' en directorio padre '%s'\n", dirName, parentDir)

	// Buscar o crear el directorio padre
	parentInodeNum, err := findOrCreateDirectoryParent(file, ext2fs, parentDir, createParents, sessionInfo)
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

	// VERIFICAR PERMISOS DE ESCRITURA EN EL DIRECTORIO PADRE
	if !session.IsRootUser() {
		if !session.CanWrite(parentInode) {
			return fmt.Errorf("sin permisos de escritura en el directorio padre '%s'", parentDir)
		}
	}

	// Verificar si el directorio ya existe
	existingInodeNum, _ := findFileInDirectory(file, ext2fs, parentInodeNum, dirName)
	if existingInodeNum != -1 {
		return fmt.Errorf("el directorio '%s' ya existe", dirPath)
	}

	// Crear el directorio en el sistema EXT2
	err = createDirectoryInEXT2System(file, ext2fs, parentInodeNum, dirName, sessionInfo, partition)
	if err != nil {
		return fmt.Errorf("error creando directorio en sistema EXT2: %s", err)
	}

	return nil
}

func createDirectoryEXT3(partition MountedPartition, dirPath string, createParents bool, sessionInfo *session.UserSession) error {
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

	parentDir, dirName := filepath.Split(dirPath)
	parentDir = strings.TrimSuffix(parentDir, "/") // Remover / final
	if parentDir == "" {
		parentDir = "/"
	}

	fmt.Printf("DEBUG: Creando directorio '%s' en directorio padre '%s'\n", dirName, parentDir)

	// Buscar o crear el directorio padre
	parentInodeNum, err := findOrCreateDirectoryParentEXT3(file, ext3fs, parentDir, createParents, sessionInfo)
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

	// VERIFICAR PERMISOS DE ESCRITURA EN EL DIRECTORIO PADRE
	if !session.IsRootUser() {
		if !session.CanWrite(parentInode) {
			return fmt.Errorf("sin permisos de escritura en el directorio padre '%s'", parentDir)
		}
	}

	// Verificar si el directorio ya existe
	existingInodeNum, _ := findFileInDirectoryEXT3ForMkdir(file, ext3fs, parentInodeNum, dirName)
	if existingInodeNum != -1 {
		return fmt.Errorf("el directorio '%s' ya existe", dirPath)
	}

	// Crear el directorio en el sistema EXT3
	err = createDirectoryInEXT3System(file, ext3fs, parentInodeNum, dirName, sessionInfo, partition)
	if err != nil {
		return fmt.Errorf("error creando directorio en sistema EXT3: %s", err)
	}

	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, dirPath, "-")

	return nil
}

// findOrCreateDirectoryParent busca o crea directorios padre (similar a MKFILE)
func findOrCreateDirectoryParent(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirPath string, createParents bool, sessionInfo *session.UserSession) (int32, error) {
	if dirPath == "/" {
		return 0, nil // Inodo raíz
	}

	// Dividir el path en componentes
	pathComponents := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentInodeNum := int32(0) // Empezar desde la raíz

	// Navegar por cada componente del path
	for i, component := range pathComponents {
		if component == "" {
			continue
		}

		// Buscar el componente en el directorio actual
		foundInodeNum, err := findFileInDirectory(file, ext2fs, currentInodeNum, component)
		if err != nil {
			return -1, fmt.Errorf("error buscando componente '%s': %s", component, err)
		}

		if foundInodeNum == -1 {
			// El directorio no existe
			if !createParents {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("el directorio padre '%s' no existe (use -p para crear directorios padre)", currentPath)
			}

			// Crear el directorio padre
			newInodeNum, err := createDirectoryInEXT2Basic(file, ext2fs, currentInodeNum, component, sessionInfo)
			if err != nil {
				return -1, fmt.Errorf("error creando directorio padre '%s': %s", component, err)
			}
			currentInodeNum = newInodeNum
			fmt.Printf("Directorio padre '%s' creado\n", component)
		} else {
			// Verificar que es un directorio
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

func findOrCreateDirectoryParentEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirPath string, createParents bool, sessionInfo *session.UserSession) (int32, error) {
	if dirPath == "/" {
		return 0, nil // Inodo raíz
	}

	// Dividir el path en componentes
	pathComponents := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentInodeNum := int32(0) // Empezar desde la raíz

	// Navegar por cada componente del path
	for i, component := range pathComponents {
		if component == "" {
			continue
		}

		// Buscar el componente en el directorio actual
		foundInodeNum, err := findFileInDirectoryEXT3ForMkdir(file, ext3fs, currentInodeNum, component)
		if err != nil {
			return -1, fmt.Errorf("error buscando componente '%s': %s", component, err)
		}

		if foundInodeNum == -1 {
			// El directorio no existe
			if !createParents {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("el directorio padre '%s' no existe (use -p para crear directorios padre)", currentPath)
			}

			// Crear el directorio padre
			newInodeNum, err := createDirectoryInEXT3Basic(file, ext3fs, currentInodeNum, component, sessionInfo)
			if err != nil {
				return -1, fmt.Errorf("error creando directorio padre '%s': %s", component, err)
			}
			currentInodeNum = newInodeNum
			fmt.Printf("Directorio padre '%s' creado\n", component)
		} else {
			// Verificar que es un directorio
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

// createDirectoryInEXT2System crea un directorio en el sistema EXT2 con actualización de superbloque
func createDirectoryInEXT2System(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession, partition MountedPartition) error {
	// Crear el directorio básico
	newInodeNum, err := createDirectoryInEXT2Basic(file, ext2fs, parentInodeNum, dirName, sessionInfo)
	if err != nil {
		return err
	}

	// Actualizar contadores del superbloque
	ext2fs.SuperBlock.S_free_inodes_count--
	ext2fs.SuperBlock.S_free_blocks_count--

	// Escribir superbloque actualizado
	err = utilities.WriteStruct(file, int64(partition.Start), ext2fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	fmt.Printf("Directorio creado:\n")
	fmt.Printf("   Nombre: %s\n", dirName)
	fmt.Printf("   Inodo: %d\n", newInodeNum)
	fmt.Printf("   Propietario: UID %d, GID %d\n", sessionInfo.UID, sessionInfo.GID)
	fmt.Printf("   Permisos: 755 (rwxr-xr-x)\n")

	return nil
}

func createDirectoryInEXT3System(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession, partition MountedPartition) error {
	// Crear el directorio básico
	newInodeNum, err := createDirectoryInEXT3Basic(file, ext3fs, parentInodeNum, dirName, sessionInfo)
	if err != nil {
		return err
	}

	// Actualizar contadores del superbloque
	ext3fs.SuperBlock.S_free_inodes_count--
	ext3fs.SuperBlock.S_free_blocks_count--

	// Escribir superbloque actualizado
	err = utilities.WriteStruct(file, int64(partition.Start), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	fmt.Printf("Directorio creado:\n")
	fmt.Printf("   Nombre: %s\n", dirName)
	fmt.Printf("   Inodo: %d\n", newInodeNum)
	fmt.Printf("   Propietario: UID %d, GID %d\n", sessionInfo.UID, sessionInfo.GID)
	fmt.Printf("   Permisos: 755 (rwxr-xr-x)\n")

	return nil
}

// createDirectoryInEXT2Basic crea un directorio básico sin actualizar superbloque (para uso interno)
func createDirectoryInEXT2Basic(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession) (int32, error) {
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

	// CREAR EL INODO DEL DIRECTORIO CON PERMISOS 664
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInode.SetFileValues(
		sessionInfo.UID,  // UID del usuario actual
		sessionInfo.GID,  // GID del usuario actual
		64,               // Tamaño típico de directorio
		0,                // Tipo: 0 = directorio
		[3]byte{7, 5, 5}, // PERMISOS 755 (rwxr-xr-x)
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

	return newInodeNum, nil
}

func findFileInDirectoryEXT3ForMkdir(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInodeNum int32, fileName string) (int32, error) {
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (dirInodeNum * ext3fs.SuperBlock.S_inode_s))
	err := utilities.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return -1, fmt.Errorf("error leyendo inodo del directorio: %s", err)
	}

	if !dirInode.IsDirectory() {
		return -1, fmt.Errorf("el inodo %d no es un directorio", dirInodeNum)
	}

	// Recorrer los bloques del directorio

	blocks := dirInode.GetIblock()

	for i := 0; i < 12 && blocks[i] != -1; i++ {
		blockNum := blocks[i]
		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err = utilities.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			return -1, fmt.Errorf("error leyendo bloque de directorio: %s", err)
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo != -1 {
				entryName := strings.TrimRight(string(dirBlock.B_content[j].B_name[:]), "\x00")
				if entryName == fileName {
					return dirBlock.B_content[j].B_inodo, nil // Encontrado
				}
			}
		}
	}

	return -1, nil // No encontrado
}

func createDirectoryInEXT3Basic(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return -1, fmt.Errorf("no hay inodos libres: %s", err)
	}

	blockNum, err := findFreeBlockEXT3(file, ext3fs)
	if err != nil {
		return -1, fmt.Errorf("no hay bloques libres: %s", err)
	}

	// CREAR EL INODO DEL DIRECTORIO CON PERMISOS 664
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInode.SetFileValues(
		sessionInfo.UID,  // UID del usuario actual
		sessionInfo.GID,  // GID del usuario actual
		64,               // Tamaño típico de directorio
		0,                // Tipo: 0 = directorio
		[3]byte{7, 5, 5}, // PERMISOS 755 (rwxr-xr-x)
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

	err = markBlockAsUsedEXT3(file, ext3fs, blockNum)
	if err != nil {
		return -1, fmt.Errorf("error marcando bloque como usado: %s", err)
	}

	// Agregar entrada al directorio padre
	err = addFileEntryToDirectoryEXT3(file, ext3fs, parentInodeNum, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %s", err)
	}

	return newInodeNum, nil
}

func updateInodeBitmapEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inodeNum int32, used bool) error {
	byteIndex := inodeNum / 8
	bitIndex := inodeNum % 8

	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_inode_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	var currentByte [1]byte
	_, err = file.Read(currentByte[:])
	if err != nil {
		return err
	}

	mask := byte(1 << bitIndex)
	if used {
		currentByte[0] |= mask // Marcar como usado
	} else {
		currentByte[0] &^= mask // Marcar como libre
	}

	_, err = file.Seek(int64(ext3fs.SuperBlock.S_bm_inode_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	_, err = file.Write(currentByte[:])
	if err != nil {
		return err
	}

	return nil
}

func markInodeAsUsedEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inodeNum int32) error {
	return updateInodeBitmapEXT3(file, ext3fs, inodeNum, true)
}

func MarkInodeAsFreeEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inodeNum int32) error {
	return updateInodeBitmapEXT3(file, ext3fs, inodeNum, false)
}

func addFileEntryToDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInodeNum int32, fileName string, fileInodeNum int32) error {
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (dirInodeNum * ext3fs.SuperBlock.S_inode_s))
	if err := utilities.ReadStruct(file, inodeOffset, dirInode); err != nil {
		return fmt.Errorf("error leyendo inodo del directorio: %s", err)
	}

	// Revisar bloques directos del directorio
	for i := 0; i < 12; i++ {
		blockNum := dirInode.Iblock[i]
		// Si el bloque no existe, crear uno y añadir la entrada
		if blockNum == -1 {
			newBlock, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para el directorio: %s", err)
			}
			// Crear bloque carpeta y agregar la entrada
			newDirBlock := bloques.NewBloqueCarpeta()
			newDirBlock.Clear()
			newDirBlock.AddEntry(fileName, fileInodeNum)

			// Escribir bloque en disco
			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (newBlock * ext3fs.SuperBlock.S_block_s))
			if err := utilities.WriteStruct(file, blockOffset, newDirBlock); err != nil {
				_ = markBlockAsFreeEXT3For(file, ext3fs, newBlock)
				return fmt.Errorf("error escribiendo nuevo bloque de directorio: %s", err)
			}
			// Marcar bloque usado en bitmap
			if err := markBlockAsUsedEXT3For(file, ext3fs, newBlock); err != nil {
				_ = markBlockAsFreeEXT3For(file, ext3fs, newBlock)
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
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
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
				newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para directorio (indirecto simple): %s", err)
				}
				newDirBlock := bloques.NewBloqueCarpeta()
				newDirBlock.Clear()
				if !newDirBlock.AddEntry(fileName, fileInodeNum) {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto simple)")
				}
				newDirOffset := int64(ext3fs.SuperBlock.S_block_start + (newDirBlockNum * ext3fs.SuperBlock.S_block_s))
				if err := utilities.WriteStruct(file, newDirOffset, newDirBlock); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto simple): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
				}

				// crear bloque apuntador y enlazar
				ptrBlock := bloques.NewBloqueApuntador()
				ptrBlock.Clear()
				ptrBlock.B_pointers[0] = newDirBlockNum
				ptrBlockNum, err = findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("no hay bloques libres para bloque de apuntadores (indirecto simple): %s", err)
				}
				ptrOffset := int64(ext3fs.SuperBlock.S_block_start + (ptrBlockNum * ext3fs.SuperBlock.S_block_s))
				if err := utilities.WriteStruct(file, ptrOffset, ptrBlock); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, ptrBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo bloque de apuntadores (indirecto simple): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, ptrBlockNum); err != nil {
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
			ptrOffset := int64(ext3fs.SuperBlock.S_block_start + (ptrBlockNum * ext3fs.SuperBlock.S_block_s))
			if err := utilities.ReadStruct(file, ptrOffset, ptrBlock); err != nil {
				return fmt.Errorf("error leyendo bloque de apuntadores (indirecto simple): %s", err)
			}

			// recorrer punteros de primer nivel
			for pIdx := 0; pIdx < 16; pIdx++ {
				p := ptrBlock.B_pointers[pIdx]
				if p == -1 {
					// crear nuevo bloque de carpeta y enlazar
					newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
					if err != nil {
						return fmt.Errorf("no hay bloques libres para agregar bloque de directorio (indirecto simple): %s", err)
					}
					newDirBlock := bloques.NewBloqueCarpeta()
					newDirBlock.Clear()
					if !newDirBlock.AddEntry(fileName, fileInodeNum) {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto simple)")
					}
					newDirOffset := int64(ext3fs.SuperBlock.S_block_start + (newDirBlockNum * ext3fs.SuperBlock.S_block_s))
					if err := utilities.WriteStruct(file, newDirOffset, newDirBlock); err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto simple): %s", err)
					}
					if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
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
				dirBlockOffset := int64(ext3fs.SuperBlock.S_block_start + (p * ext3fs.SuperBlock.S_block_s))
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
				newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para directorio (indirecto doble): %s", err)
				}
				newDirBlock := bloques.NewBloqueCarpeta()
				newDirBlock.Clear()
				if !newDirBlock.AddEntry(fileName, fileInodeNum) {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto doble)")
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto doble): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
					return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
				}

				// crear nivel2 (apuntadores) que apunte al bloque de carpeta
				level2 := bloques.NewBloqueApuntador()
				level2.Clear()
				level2.B_pointers[0] = newDirBlockNum
				level2Num, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("no hay bloques libres para nivel2 apuntadores (indirecto doble): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(level2Num*ext3fs.SuperBlock.S_block_s)), level2); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, level2Num)
					return fmt.Errorf("error escribiendo nivel2 apuntadores (indirecto doble): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, level2Num); err != nil {
					return fmt.Errorf("error marcando nivel2 apuntadores usados: %s", err)
				}

				// crear nivel1 que apunte al nivel2
				level1 := bloques.NewBloqueApuntador()
				level1.Clear()
				level1.B_pointers[0] = level2Num
				level1Num, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, level2Num)
					return fmt.Errorf("no hay bloques libres para nivel1 apuntadores (indirecto doble): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(level1Num*ext3fs.SuperBlock.S_block_s)), level1); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, level2Num)
					_ = markBlockAsFreeEXT3For(file, ext3fs, level1Num)
					return fmt.Errorf("error escribiendo nivel1 apuntadores (indirecto doble): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, level1Num); err != nil {
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
			level1Offset := int64(ext3fs.SuperBlock.S_block_start + (ptrLevel1Num * ext3fs.SuperBlock.S_block_s))
			if err := utilities.ReadStruct(file, level1Offset, level1); err != nil {
				return fmt.Errorf("error leyendo nivel1 apuntadores (indirecto doble): %s", err)
			}
			for idx1 := 0; idx1 < 16; idx1++ {
				level2Num := level1.B_pointers[idx1]
				if level2Num == -1 {
					// crear level2 + nuevo bloque carpeta
					newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
					if err != nil {
						return fmt.Errorf("no hay bloques libres para directorio (indirecto doble, crear level2): %s", err)
					}
					newDirBlock := bloques.NewBloqueCarpeta()
					newDirBlock.Clear()
					if !newDirBlock.AddEntry(fileName, fileInodeNum) {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto doble)")
					}
					if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto doble): %s", err)
					}
					if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
						return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
					}

					level2 := bloques.NewBloqueApuntador()
					level2.Clear()
					level2.B_pointers[0] = newDirBlockNum
					level2Num, err := findFreeBlockEXT3(file, ext3fs)
					if err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("no hay bloques libres para level2 apuntadores (indirecto doble): %s", err)
					}
					if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(level2Num*ext3fs.SuperBlock.S_block_s)), level2); err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						_ = markBlockAsFreeEXT3For(file, ext3fs, level2Num)
						return fmt.Errorf("error escribiendo level2 apuntadores (indirecto doble): %s", err)
					}
					if err := markBlockAsUsedEXT3For(file, ext3fs, level2Num); err != nil {
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
				level2Offset := int64(ext3fs.SuperBlock.S_block_start + (level2Num * ext3fs.SuperBlock.S_block_s))
				if err := utilities.ReadStruct(file, level2Offset, level2); err != nil {
					return fmt.Errorf("error leyendo level2 apuntadores (indirecto doble): %s", err)
				}
				for idx2 := 0; idx2 < 16; idx2++ {
					p := level2.B_pointers[idx2]
					if p == -1 {
						// crear nuevo bloque carpeta y enlazar
						newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
						if err != nil {
							return fmt.Errorf("no hay bloques libres para agregar bloque de directorio (indirecto doble): %s", err)
						}
						newDirBlock := bloques.NewBloqueCarpeta()
						newDirBlock.Clear()
						if !newDirBlock.AddEntry(fileName, fileInodeNum) {
							_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
							return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto doble)")
						}
						if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
							_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
							return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto doble): %s", err)
						}
						if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
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
					dirBlockOffset := int64(ext3fs.SuperBlock.S_block_start + (p * ext3fs.SuperBlock.S_block_s))
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
				newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para directorio (indirecto triple): %s", err)
				}
				newDirBlock := bloques.NewBloqueCarpeta()
				newDirBlock.Clear()
				if !newDirBlock.AddEntry(fileName, fileInodeNum) {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
					return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
				}

				// crear nivel3 que apunte al bloque carpeta
				l3 := bloques.NewBloqueApuntador()
				l3.Clear()
				l3.B_pointers[0] = newDirBlockNum
				l3Num, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					return fmt.Errorf("no hay bloques libres para nivel3 apuntadores (indirecto triple): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(l3Num*ext3fs.SuperBlock.S_block_s)), l3); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
					return fmt.Errorf("error escribiendo nivel3 apuntadores (indirecto triple): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, l3Num); err != nil {
					return fmt.Errorf("error marcando nivel3 apuntadores usados: %s", err)
				}

				// crear nivel2 que apunte al nivel3
				l2 := bloques.NewBloqueApuntador()
				l2.Clear()
				l2.B_pointers[0] = l3Num
				l2Num, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
					return fmt.Errorf("no hay bloques libres para nivel2 apuntadores (indirecto triple): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(l2Num*ext3fs.SuperBlock.S_block_s)), l2); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l2Num)
					return fmt.Errorf("error escribiendo nivel2 apuntadores (indirecto triple): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, l2Num); err != nil {
					return fmt.Errorf("error marcando nivel2 apuntadores usados: %s", err)
				}

				// crear nivel1 que apunte al nivel2
				l1 := bloques.NewBloqueApuntador()
				l1.Clear()
				l1.B_pointers[0] = l2Num
				l1Num, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l2Num)
					return fmt.Errorf("no hay bloques libres para nivel1 apuntadores (indirecto triple): %s", err)
				}
				if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(l1Num*ext3fs.SuperBlock.S_block_s)), l1); err != nil {
					_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l2Num)
					_ = markBlockAsFreeEXT3For(file, ext3fs, l1Num)
					return fmt.Errorf("error escribiendo nivel1 apuntadores (indirecto triple): %s", err)
				}
				if err := markBlockAsUsedEXT3For(file, ext3fs, l1Num); err != nil {
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
			l1Offset := int64(ext3fs.SuperBlock.S_block_start + (ptrL1 * ext3fs.SuperBlock.S_block_s))
			if err := utilities.ReadStruct(file, l1Offset, l1); err != nil {
				return fmt.Errorf("error leyendo nivel1 apuntadores (indirecto triple): %s", err)
			}
			for idx1 := 0; idx1 < 16; idx1++ {
				l2Num := l1.B_pointers[idx1]
				if l2Num == -1 {
					// crear l2 + l3 + bloque carpeta
					newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
					if err != nil {
						return fmt.Errorf("no hay bloques libres para directorio (indirecto triple, crear l2): %s", err)
					}
					newDirBlock := bloques.NewBloqueCarpeta()
					newDirBlock.Clear()
					if !newDirBlock.AddEntry(fileName, fileInodeNum) {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
					}
					if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
					}
					if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
						return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
					}

					l3 := bloques.NewBloqueApuntador()
					l3.Clear()
					l3.B_pointers[0] = newDirBlockNum
					l3Num, err := findFreeBlockEXT3(file, ext3fs)
					if err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						return fmt.Errorf("no hay bloques libres para l3 apuntadores (indirecto triple): %s", err)
					}
					if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(l3Num*ext3fs.SuperBlock.S_block_s)), l3); err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
						return fmt.Errorf("error escribiendo l3 apuntadores (indirecto triple): %s", err)
					}
					if err := markBlockAsUsedEXT3For(file, ext3fs, l3Num); err != nil {
						return fmt.Errorf("error marcando l3 apuntadores usados: %s", err)
					}

					l2 := bloques.NewBloqueApuntador()
					l2.Clear()
					l2.B_pointers[0] = l3Num
					l2Num, err := findFreeBlockEXT3(file, ext3fs)
					if err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
						return fmt.Errorf("no hay bloques libres para l2 apuntadores (indirecto triple): %s", err)
					}
					if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(l2Num*ext3fs.SuperBlock.S_block_s)), l2); err != nil {
						_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
						_ = markBlockAsFreeEXT3For(file, ext3fs, l3Num)
						_ = markBlockAsFreeEXT3For(file, ext3fs, l2Num)
						return fmt.Errorf("error escribiendo l2 apuntadores (indirecto triple): %s", err)
					}
					if err := markBlockAsUsedEXT3For(file, ext3fs, l2Num); err != nil {
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
				l2Offset := int64(ext3fs.SuperBlock.S_block_start + (l2Num * ext3fs.SuperBlock.S_block_s))
				if err := utilities.ReadStruct(file, l2Offset, l2); err != nil {
					return fmt.Errorf("error leyendo l2 apuntadores (indirecto triple): %s", err)
				}
				for idx2 := 0; idx2 < 16; idx2++ {
					l3Num := l2.B_pointers[idx2]
					if l3Num == -1 {
						// crear l3 + nuevo bloque carpeta
						newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
						if err != nil {
							return fmt.Errorf("no hay bloques libres para directorio (indirecto triple, crear l3): %s", err)
						}
						newDirBlock := bloques.NewBloqueCarpeta()
						newDirBlock.Clear()
						if !newDirBlock.AddEntry(fileName, fileInodeNum) {
							_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
							return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
						}
						if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
							_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
							return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
						}
						if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
							return fmt.Errorf("error marcando nuevo bloque de directorio usado: %s", err)
						}

						l3 := bloques.NewBloqueApuntador()
						l3.Clear()
						l3.B_pointers[0] = newDirBlockNum
						l3NumNew, err := findFreeBlockEXT3(file, ext3fs)
						if err != nil {
							_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
							return fmt.Errorf("no hay bloques libres para l3 apuntadores (indirecto triple): %s", err)
						}
						if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(l3NumNew*ext3fs.SuperBlock.S_block_s)), l3); err != nil {
							_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
							_ = markBlockAsFreeEXT3For(file, ext3fs, l3NumNew)
							return fmt.Errorf("error escribiendo l3 apuntadores (indirecto triple): %s", err)
						}
						if err := markBlockAsUsedEXT3For(file, ext3fs, l3NumNew); err != nil {
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
					l3Offset := int64(ext3fs.SuperBlock.S_block_start + (l3Num * ext3fs.SuperBlock.S_block_s))
					if err := utilities.ReadStruct(file, l3Offset, l3); err != nil {
						return fmt.Errorf("error leyendo l3 apuntadores (indirecto triple): %s", err)
					}
					for idx3 := 0; idx3 < 16; idx3++ {
						p := l3.B_pointers[idx3]
						if p == -1 {
							// crear bloque carpeta y enlazar
							newDirBlockNum, err := findFreeBlockEXT3(file, ext3fs)
							if err != nil {
								return fmt.Errorf("no hay bloques libres para agregar bloque de directorio (indirecto triple): %s", err)
							}
							newDirBlock := bloques.NewBloqueCarpeta()
							newDirBlock.Clear()
							if !newDirBlock.AddEntry(fileName, fileInodeNum) {
								_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
								return fmt.Errorf("no se pudo agregar entrada al nuevo bloque de directorio (indirecto triple)")
							}
							if err := utilities.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+(newDirBlockNum*ext3fs.SuperBlock.S_block_s)), newDirBlock); err != nil {
								_ = markBlockAsFreeEXT3For(file, ext3fs, newDirBlockNum)
								return fmt.Errorf("error escribiendo nuevo bloque de directorio (indirecto triple): %s", err)
							}
							if err := markBlockAsUsedEXT3For(file, ext3fs, newDirBlockNum); err != nil {
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
						dirBlockOffset := int64(ext3fs.SuperBlock.S_block_start + (p * ext3fs.SuperBlock.S_block_s))
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

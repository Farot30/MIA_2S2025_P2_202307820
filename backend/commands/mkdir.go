package commands

import (
	bloques "MIA_2S2025_P1_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	session "MIA_2S2025_P1_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"MIA_2S2025_P1_202307820/utilities"
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

	// Crear el directorio
	err := createDirectory(mountedPartition, dirPath, createParents, sessionInfo)
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
	fmt.Printf("   Permisos: 664 (rw-rw-r--)\n")

	return nil
}

// createDirectoryInEXT2Basic crea un directorio básico sin actualizar superbloque (para uso interno)
func createDirectoryInEXT2Basic(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, parentInodeNum int32, dirName string, sessionInfo *session.UserSession) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := findFreeInode(file, ext2fs)
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
		[3]byte{6, 6, 4}, // PERMISOS 664 (rw-rw-r--)
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

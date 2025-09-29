package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func CHMOD(args []string) {
	// Verificar que hay una sesión activa
	if !sessionP2.IsSessionActive() {
		fmt.Printf("ERROR: No active session. Please log in first.\n")
		return
	}

	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)
	targetPath := strings.Trim(params["path"], "\"")
	ugoPermissions := strings.Trim(params["ugo"], "\"")
	recursive := utilitiesP2.HasParam(params, "r")

	// Validar parámetros obligatorios
	if targetPath == "" {
		fmt.Println("ERROR: Falta el parámetro obligatorio 'path'.")
		fmt.Println("Uso: chmod -path=<ruta> -ugo=<permisos> [-r]")
		return
	}

	if ugoPermissions == "" {
		fmt.Println("ERROR: Falta el parámetro obligatorio 'ugo'.")
		fmt.Println("Uso: chmod -path=<ruta> -ugo=<permisos> [-r]")
		return
	}

	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(targetPath, "/") {
		fmt.Println("ERROR: La ruta debe ser absoluta y comenzar con '/'.")
		return
	}

	// Validar formato de permisos UGO
	if len(ugoPermissions) != 3 {
		fmt.Println("ERROR: Los permisos deben tener exactamente 3 dígitos (UGO).")
		fmt.Println("Ejemplo: -ugo=764")
		return
	}

	// Validar y parsear permisos
	permissions, err := parseUGOPermissions(ugoPermissions)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	// Obtener información de la sesión
	sessionInfo := sessionP2.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Println("ERROR: No se pudo obtener información de la sesión actual.")
		return
	}

	// Obtener la partición montada
	mountedPartition, exist := GetMountedPartition(sessionInfo.PartitionID)
	if !exist {
		fmt.Println("ERROR: La partición no está montada.")
		return
	}

	// Determinar tipo de sistema de archivos
	fileSystemType, err := getFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	// Ejecutar el cambio de permisos según el tipo de sistema
	if fileSystemType == EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE {
		err = chmodInEXT2(mountedPartition, targetPath, permissions, recursive, sessionInfo)
	} else if fileSystemType == EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		err = chmodInEXT3(mountedPartition, targetPath, permissions, recursive, sessionInfo)
	} else {
		fmt.Println("ERROR: Tipo de sistema de archivos no soportado.")
		return
	}

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	if recursive {
		fmt.Printf("Permisos cambiados recursivamente a '%s' para '%s'\n", ugoPermissions, targetPath)
	} else {
		fmt.Printf("Permisos cambiados a '%s' para '%s'\n", ugoPermissions, targetPath)
	}
}

// parseUGOPermissions valida y parsea los permisos en formato UGO
func parseUGOPermissions(ugoStr string) ([3]byte, error) {
	var permissions [3]byte

	for i, char := range ugoStr {
		digit, err := strconv.Atoi(string(char))
		if err != nil || digit < 0 || digit > 7 {
			return permissions, fmt.Errorf("permiso '%c' inválido. Debe ser un dígito entre 0 y 7", char)
		}
		permissions[i] = byte(digit)
	}

	return permissions, nil
}

// getFileSystemType determina el tipo de sistema de archivos
func getFileSystemType(partition MountedPartition) (int32, error) {
	file, err := os.Open(partition.Path)
	if err != nil {
		return 0, fmt.Errorf("error abriendo la partición: %v", err)
	}
	defer file.Close()

	// Leer el superbloque
	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(partition.Start), superBlock)
	if err != nil {
		return 0, fmt.Errorf("error leyendo superbloque: %v", err)
	}

	return superBlock.S_filesystem_type, nil
}

// chmodInEXT2 maneja el cambio de permisos en sistema EXT2
func chmodInEXT2(partition MountedPartition, targetPath string, permissions [3]byte, recursive bool, sessionInfo *sessionP2.UserSession) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT2: %v", err)
	}

	return executeChmod(file, ext2fs.SuperBlock, targetPath, permissions, recursive, sessionInfo, nil)
}

// chmodInEXT3 maneja el cambio de permisos en sistema EXT3 con journaling
func chmodInEXT3(partition MountedPartition, targetPath string, permissions [3]byte, recursive bool, sessionInfo *sessionP2.UserSession) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT3: %v", err)
	}

	// Registrar operación en el journal antes de ejecutar
	permissionsStr := fmt.Sprintf("%d%d%d", permissions[0], permissions[1], permissions[2])
	if recursive {
		permissionsStr += " -r"
	}

	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_CHMOD, targetPath, permissionsStr)
	if err != nil {
		return fmt.Errorf("error registrando operación en journal: %v", err)
	}

	// Ejecutar el cambio de permisos
	err = executeChmod(file, ext3fs.SuperBlock, targetPath, permissions, recursive, sessionInfo, ext3fs)
	if err != nil {
		// Registrar error en journal
		ext3fs.AddJournalEntry("chmod_error", targetPath, err.Error())
		return err
	}

	// Registrar éxito en journal
	err = ext3fs.AddJournalEntry("chmod_success", targetPath, permissionsStr)
	if err != nil {
		fmt.Printf("WARNING: No se pudo registrar éxito en journal: %v\n", err)
	}

	return nil
}

// executeChmod ejecuta el cambio de permisos común para EXT2 y EXT3
func executeChmod(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, targetPath string, permissions [3]byte, recursive bool, sessionInfo *sessionP2.UserSession, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	// Separar directorio padre y nombre del elemento
	targetDir := filepath.Dir(targetPath)
	targetName := filepath.Base(targetPath)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT2ForChmod(file, superBlock, targetDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio '%s': %v", targetDir, err)
	}

	// Leer inodo del directorio padre
	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+parentInodeNum*superBlock.S_inode_s), parentInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre: %v", err)
	}

	// Buscar el elemento objetivo
	targetInodeNum, err := findFileInDirectoryEXT2ForChmod(file, superBlock, parentInode, targetName)
	if err != nil || targetInodeNum == -1 {
		return fmt.Errorf("no se pudo encontrar '%s' en '%s'", targetName, targetDir)
	}

	// Leer el inodo del elemento objetivo
	targetInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+targetInodeNum*superBlock.S_inode_s), targetInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento objetivo: %v", err)
	}

	// Verificar permisos
	err = checkChmodPermissions(targetInode, sessionInfo)
	if err != nil {
		return err
	}

	// Cambiar permisos
	if recursive && targetInode.Itype == 0 { // Es directorio y es recursivo
		return chmodRecursive(file, superBlock, targetInode, targetInodeNum, permissions, sessionInfo, ext3fs, targetPath)
	} else {
		return chmodSingle(file, superBlock, targetInode, targetInodeNum, permissions)
	}
}

// checkChmodPermissions verifica si el usuario puede cambiar los permisos
func checkChmodPermissions(targetInode *EstructuraarchivocarpetaP2.Inodo, sessionInfo *sessionP2.UserSession) error {
	// Root puede cambiar permisos de cualquier archivo
	if sessionP2.IsRootUser() {
		return nil
	}

	// Otros usuarios solo pueden cambiar permisos de sus propios archivos
	if targetInode.Iuid != sessionInfo.UID {
		return fmt.Errorf("no tiene permisos para cambiar los permisos. Solo el propietario o root pueden hacerlo")
	}

	return nil
}

// chmodSingle cambia los permisos de un solo archivo/directorio
func chmodSingle(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, targetInode *EstructuraarchivocarpetaP2.Inodo, targetInodeNum int32, permissions [3]byte) error {
	// Cambiar los permisos del inodo
	targetInode.Iperm = permissions
	targetInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp() // Actualizar tiempo de modificación

	// Escribir el inodo actualizado
	err := utilitiesP2.WriteStruct(file, int64(superBlock.S_inode_start+targetInodeNum*superBlock.S_inode_s), targetInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo actualizado: %v", err)
	}

	return nil
}

// chmodRecursive cambia los permisos recursivamente en un directorio
func chmodRecursive(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum int32, permissions [3]byte, sessionInfo *sessionP2.UserSession, ext3fs *EstructuraExt3P2.EXT3FileSystem, basePath string) error {
	// Cambiar permisos del directorio actual
	err := chmodSingle(file, superBlock, dirInode, dirInodeNum, permissions)
	if err != nil {
		return fmt.Errorf("error cambiando permisos del directorio: %v", err)
	}

	// Procesar todas las entradas del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_block_start+blockNum*superBlock.S_block_s), block)
		if err != nil {
			fmt.Printf("[WARNING] No se pudo leer bloque %d: %v\n", blockNum, err)
			continue
		}

		// Procesar cada entrada del bloque
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)

				// Saltar entradas . y ..
				if entryName == "." || entryName == ".." {
					continue
				}

				// Construir la ruta completa
				entryPath := basePath + "/" + entryName
				if basePath == "/" {
					entryPath = "/" + entryName
				}

				// Leer el inodo de la entrada
				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+entryInodeNum*superBlock.S_inode_s), entryInode)
				if err != nil {
					fmt.Printf("[WARNING] No se pudo leer inodo de '%s': %v\n", entryName, err)
					continue
				}

				// Verificar permisos antes de cambiar
				err = checkChmodPermissions(entryInode, sessionInfo)
				if err != nil {
					fmt.Printf("[WARNING] Sin permisos para cambiar '%s': %v\n", entryName, err)
					continue
				}

				// Registrar en journal si es EXT3
				if ext3fs != nil {
					permissionsStr := fmt.Sprintf("%d%d%d", permissions[0], permissions[1], permissions[2])
					ext3fs.AddJournalEntry("chmod_recursive", entryPath, permissionsStr)
				}

				if entryInode.Itype == 0 { // Es directorio
					// Cambiar recursivamente
					err = chmodRecursive(file, superBlock, entryInode, entryInodeNum, permissions, sessionInfo, ext3fs, entryPath)
					if err != nil {
						fmt.Printf("[WARNING] Error en subdirectorio '%s': %v\n", entryName, err)
					}
				} else { // Es archivo
					// Cambiar permisos del archivo
					err = chmodSingle(file, superBlock, entryInode, entryInodeNum, permissions)
					if err != nil {
						fmt.Printf("[WARNING] Error cambiando permisos de '%s': %v\n", entryName, err)
					}
				}
			}
		}
	}

	return nil
}

// findDirectoryInEXT2ForChmod busca un directorio en el sistema EXT2
func findDirectoryInEXT2ForChmod(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, path string) (int32, error) {
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
		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+currentInodeNum*superBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo del directorio: %v", err)
		}

		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		nextInodeNum, err := findFileInDirectoryEXT2ForChmod(file, superBlock, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findFileInDirectoryEXT2ForChmod busca un archivo o directorio en un directorio específico
func findFileInDirectoryEXT2ForChmod(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en bloques directos
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_block_start+blockNum*superBlock.S_block_s), block)
		if err != nil {
			continue
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)
				if entryName == fileName {
					return entryInodeNum, nil
				}
			}
		}
	}

	return -1, fmt.Errorf("no se encontró '%s' en el directorio", fileName)
}

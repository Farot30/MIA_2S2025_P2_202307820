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
	"strings"
)

func CHOWN(args []string) {
	// Verificar que hay una sesión activa
	if !sessionP2.IsSessionActive() {
		fmt.Printf("ERROR: No active session. Please log in first.\n")
		return
	}

	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)
	targetPath := strings.Trim(params["path"], "\"")
	newOwner := strings.Trim(params["usuario"], "\"")
	recursive := utilitiesP2.HasParam(params, "r")

	// Validar parámetros obligatorios
	if targetPath == "" {
		fmt.Println("ERROR: Falta el parámetro obligatorio 'path'.")
		fmt.Println("Uso: chown -path=<ruta> -usuario=<nuevo_propietario> [-r]")
		return
	}

	if newOwner == "" {
		fmt.Println("ERROR: Falta el parámetro obligatorio 'usuario'.")
		fmt.Println("Uso: chown -path=<ruta> -usuario=<nuevo_propietario> [-r]")
		return
	}

	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(targetPath, "/") {
		fmt.Println("ERROR: La ruta debe ser absoluta y comenzar con '/'.")
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

	fileSystemType, err := getFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	if fileSystemType == EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE {
		err = chownInEXT2(mountedPartition, targetPath, newOwner, recursive, sessionInfo)
	} else if fileSystemType == EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		err = chownInEXT3(mountedPartition, targetPath, newOwner, recursive, sessionInfo)
	} else {
		fmt.Printf("ERROR: Tipo de sistema de archivos no soportado.\n")
	}

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	if recursive {
		fmt.Printf("Propietario cambiado recursivamente a '%s' para '%s'\n", newOwner, targetPath)
	} else {
		fmt.Printf("Propietario cambiado a '%s' para '%s'\n", newOwner, targetPath)
	}
}

// chownInEXT2 maneja el cambio de propietario en sistema EXT2
func chownInEXT2(partition MountedPartition, targetPath, newOwner string, recursive bool, sessionInfo *sessionP2.UserSession) error {
	// Abrir archivo de la partición
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	// Leer el sistema de archivos EXT2
	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT2: %v", err)
	}

	// Validar que el nuevo propietario existe
	newOwnerUID, err := getUserUID(partition, newOwner)
	if err != nil {
		return fmt.Errorf("el usuario '%s' no existe: %v", newOwner, err)
	}

	// Separar directorio padre y nombre del elemento
	targetDir := filepath.Dir(targetPath)
	targetName := filepath.Base(targetPath)

	// Buscar el directorio padre
	parentInodeNum, err := findDirectoryInEXT2ForChown(file, ext2fs, targetDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio '%s': %v", targetDir, err)
	}

	// Leer inodo del directorio padre
	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+parentInodeNum*ext2fs.SuperBlock.S_inode_s), parentInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre: %v", err)
	}

	// Buscar el elemento objetivo
	targetInodeNum, err := findFileInDirectoryEXT2ForChown(file, ext2fs, parentInode, targetName)
	if err != nil || targetInodeNum == -1 {
		return fmt.Errorf("no se pudo encontrar '%s' en '%s'", targetName, targetDir)
	}

	// Leer el inodo del elemento objetivo
	targetInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+targetInodeNum*ext2fs.SuperBlock.S_inode_s), targetInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento objetivo: %v", err)
	}

	// Verificar permisos
	err = checkChownPermissions(targetInode, sessionInfo)
	if err != nil {
		return err
	}

	// Cambiar propietario
	if recursive && targetInode.Itype == 0 { // Es directorio y es recursivo
		return chownRecursive(file, ext2fs, targetInode, targetInodeNum, newOwnerUID, sessionInfo)
	} else {
		return chownSingle(file, ext2fs, targetInode, targetInodeNum, newOwnerUID)
	}
}

func chownInEXT3(partition MountedPartition, targetPath, newOwner string, recursive bool, sessionInfo *sessionP2.UserSession) error {

	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT3: %v", err)
	}

	newOwnerUID, err := getUserUID(partition, newOwner)
	if err != nil {
		return fmt.Errorf("el usuario '%s' no existe: %v", newOwner, err)
	}

	targetDir := filepath.Dir(targetPath)
	targetName := filepath.Base(targetPath)

	parentInodeNum, err := findDirectoryInEXT3ForChown(file, ext3fs, targetDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio '%s': %v", targetDir, err)
	}

	parentInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre: %v", err)
	}

	targetInodeNum, err := findFileInDirectoryEXT3ForChown(file, ext3fs, parentInode, targetName)
	if err != nil || targetInodeNum == -1 {
		return fmt.Errorf("no se pudo encontrar '%s' en '%s'", targetName, targetDir)
	}

	targetInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+targetInodeNum*ext3fs.SuperBlock.S_inode_s), targetInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento objetivo: %v", err)
	}

	err = checkChownPermissions(targetInode, sessionInfo)
	if err != nil {
		return err
	}

	if recursive && targetInode.Itype == 0 {
		err = chownRecursiveEXT3(file, ext3fs, targetInode, targetInodeNum, newOwnerUID, sessionInfo)
	} else {
		err = chownSingleEXT3(file, ext3fs, targetInode, targetInodeNum, newOwnerUID)
	}
	if err != nil {
		return err
	}

	// Registrar la operación en el journal
	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_CHOWN, targetPath, fmt.Sprintf("Nuevo propietario: %s", newOwner))
	if err != nil {
		fmt.Printf("Advertencia: No se pudo registrar la operación en el journal: %v\n", err)
	}

	return nil
}

// checkChownPermissions verifica si el usuario actual puede cambiar el propietario
func checkChownPermissions(targetInode *EstructuraarchivocarpetaP2.Inodo, sessionInfo *sessionP2.UserSession) error {
	// Root puede cambiar cualquier propietario
	if sessionP2.IsRootUser() {
		return nil
	}

	// Otros usuarios solo pueden cambiar propietarios de sus propios archivos
	if targetInode.Iuid != sessionInfo.UID {
		return fmt.Errorf("no tiene permisos para cambiar el propietario. Solo el propietario actual o root pueden hacerlo")
	}

	return nil
}

// chownSingle cambia el propietario de un solo archivo/directorio
func chownSingle(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, targetInode *EstructuraarchivocarpetaP2.Inodo, targetInodeNum, newOwnerUID int32) error {
	// Cambiar el UID del inodo
	targetInode.Iuid = newOwnerUID
	targetInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp() // Actualizar tiempo de modificación

	// Escribir el inodo actualizado
	err := utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+targetInodeNum*ext2fs.SuperBlock.S_inode_s), targetInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo actualizado: %v", err)
	}

	return nil
}

func chownSingleEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, targetInode *EstructuraarchivocarpetaP2.Inodo, targetInodeNum, newOwnerUID int32) error {
	// Cambiar el UID del inodo
	targetInode.Iuid = newOwnerUID
	targetInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp() // Actualizar tiempo de modificación

	// Escribir el inodo actualizado
	err := utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+targetInodeNum*ext3fs.SuperBlock.S_inode_s), targetInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo actualizado: %v", err)
	}

	return nil
}

// chownRecursive cambia el propietario recursivamente en un directorio
func chownRecursive(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum, newOwnerUID int32, sessionInfo *sessionP2.UserSession) error {
	// Cambiar propietario del directorio actual
	err := chownSingle(file, ext2fs, dirInode, dirInodeNum, newOwnerUID)
	if err != nil {
		return fmt.Errorf("error cambiando propietario del directorio: %v", err)
	}

	// Procesar todas las entradas del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
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

				// Leer el inodo de la entrada
				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+entryInodeNum*ext2fs.SuperBlock.S_inode_s), entryInode)
				if err != nil {
					fmt.Printf("[WARNING] No se pudo leer inodo de '%s': %v\n", entryName, err)
					continue
				}

				// Verificar permisos antes de cambiar
				err = checkChownPermissions(entryInode, sessionInfo)
				if err != nil {
					fmt.Printf("[WARNING] Sin permisos para cambiar '%s': %v\n", entryName, err)
					continue
				}

				if entryInode.Itype == 0 { // Es directorio
					// Cambiar recursivamente
					err = chownRecursive(file, ext2fs, entryInode, entryInodeNum, newOwnerUID, sessionInfo)
					if err != nil {
						fmt.Printf("[WARNING] Error en subdirectorio '%s': %v\n", entryName, err)
					}
				} else { // Es archivo
					// Cambiar propietario del archivo
					err = chownSingle(file, ext2fs, entryInode, entryInodeNum, newOwnerUID)
					if err != nil {
						fmt.Printf("[WARNING] Error cambiando propietario de '%s': %v\n", entryName, err)
					}
				}
			}
		}
	}

	return nil
}

func chownRecursiveEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum, newOwnerUID int32, sessionInfo *sessionP2.UserSession) error {
	// Cambiar propietario del directorio actual
	err := chownSingleEXT3(file, ext3fs, dirInode, dirInodeNum, newOwnerUID)
	if err != nil {
		return fmt.Errorf("error cambiando propietario del directorio: %v", err)
	}

	// Procesar todas las entradas del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
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

				// Leer el inodo de la entrada
				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+entryInodeNum*ext3fs.SuperBlock.S_inode_s), entryInode)
				if err != nil {
					fmt.Printf("[WARNING] No se pudo leer inodo de '%s': %v\n", entryName, err)
					continue
				}

				// Verificar permisos antes de cambiar
				err = checkChownPermissions(entryInode, sessionInfo)
				if err != nil {
					fmt.Printf("[WARNING] Sin permisos para cambiar '%s': %v\n", entryName, err)
					continue
				}

				if entryInode.Itype == 0 { // Es directorio
					// Cambiar recursivamente
					err = chownRecursiveEXT3(file, ext3fs, entryInode, entryInodeNum, newOwnerUID, sessionInfo)
					if err != nil {
						fmt.Printf("[WARNING] Error en subdirectorio '%s': %v\n", entryName, err)
					}
				} else { // Es archivo
					// Cambiar propietario del archivo
					err = chownSingleEXT3(file, ext3fs, entryInode, entryInodeNum, newOwnerUID)
					if err != nil {
						fmt.Printf("[WARNING] Error cambiando propietario de '%s': %v\n", entryName, err)
					}
				}
			}
		}
	}

	return nil
}

// getUserUID obtiene el UID de un usuario por su nombre
func getUserUID(partition MountedPartition, username string) (int32, error) {
	// Leer el archivo users.txt
	usersContent, err := readUsersFileForChown(partition)
	if err != nil {
		return -1, fmt.Errorf("error leyendo archivo de usuarios: %v", err)
	}

	// Parsear el contenido
	users, _ := parseUsersFileForChown(usersContent)

	// Buscar el usuario
	for _, user := range users {
		if strings.EqualFold(user.Username, username) {
			if user.UID == 0 { // Usuario eliminado
				return -1, fmt.Errorf("usuario '%s' está marcado como eliminado", username)
			}
			return user.UID, nil
		}
	}

	return -1, fmt.Errorf("usuario '%s' no encontrado", username)
}

// readUsersFileForChown lee el archivo users.txt de la partición
func readUsersFileForChown(partition MountedPartition) (string, error) {
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return "", err
	}
	defer file.Close()

	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return "", err
	}

	// Buscar users.txt en el directorio raíz
	rootInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start), rootInode)
	if err != nil {
		return "", err
	}

	usersInodeNum, err := findFileInDirectoryEXT2ForChown(file, ext2fs, rootInode, "users.txt")
	if err != nil {
		return "", err
	}

	usersInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+usersInodeNum*ext2fs.SuperBlock.S_inode_s), usersInode)
	if err != nil {
		return "", err
	}

	// Leer contenido del archivo
	content, err := ReadFileContentCorrect(file, ext2fs, usersInode)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// parseUsersFileForChown parsea el contenido del archivo users.txt
func parseUsersFileForChown(content string) ([]UserChown, []GroupChown) {
	var users []UserChown
	var groups []GroupChown

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}

		// Parsear ID
		var id int32
		fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &id)

		recordType := strings.TrimSpace(parts[1])

		if recordType == "G" && len(parts) >= 3 { // Grupo
			groupName := strings.TrimSpace(parts[2])
			groups = append(groups, GroupChown{
				GID:  id,
				Name: groupName,
			})
		} else if recordType == "U" && len(parts) >= 5 { // Usuario
			groupName := strings.TrimSpace(parts[2])
			username := strings.TrimSpace(parts[3])
			users = append(users, UserChown{
				UID:      id,
				Username: username,
				Group:    groupName,
			})
		}
	}

	return users, groups
}

// findDirectoryInEXT2ForChown busca un directorio en el sistema EXT2
func findDirectoryInEXT2ForChown(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, path string) (int32, error) {
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
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+currentInodeNum*ext2fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo del directorio: %v", err)
		}

		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		nextInodeNum, err := findFileInDirectoryEXT2ForChown(file, ext2fs, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

func findDirectoryInEXT3ForChown(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
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
			return -1, fmt.Errorf("error leyendo inodo del directorio: %v", err)
		}

		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		nextInodeNum, err := findFileInDirectoryEXT3ForChown(file, ext3fs, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findFileInDirectoryEXT2ForChown busca un archivo o directorio en un directorio específico
func findFileInDirectoryEXT2ForChown(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en bloques directos
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
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

func findFileInDirectoryEXT3ForChown(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en bloques directos
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
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

// UserChown representa un usuario para el comando chown
type UserChown struct {
	UID      int32
	Username string
	Group    string
}

// GroupChown representa un grupo para el comando chown
type GroupChown struct {
	GID  int32
	Name string
}

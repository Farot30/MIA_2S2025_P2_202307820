package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	structuresP2 "MIA_2S2025_P2_202307820/structures"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
)

// LOGIN implementa el comando para iniciar sesión en el sistema
func LOGIN(args []string) {
	// Parsear parámetros usando utilities
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetros requeridos
	required := []string{"user", "pass", "id"}
	if err := utilitiesP2.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		fmt.Println("Uso: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	// Obtener parámetros
	user := strings.Trim(params["user"], "\"")
	pass := strings.Trim(params["pass"], "\"")
	id := strings.ToUpper(params["id"])

	// Verificar que no haya una sesión activa
	if session.IsSessionActive() {
		fmt.Println("Error: Ya existe una sesión activa. Debe hacer LOGOUT primero")
		currentSession := session.GetCurrentSession()
		fmt.Printf("Sesión actual: Usuario '%s' en partición '%s'\n",
			currentSession.Username, currentSession.PartitionID)
		return
	}

	// Verificar que la partición esté montada
	mountedPartition, exists := GetMountedPartition(id)
	if !exists {
		fmt.Printf("Error: La partición con ID '%s' no está montada\n", id)
		fmt.Println("Use el comando 'mounted' para ver las particiones montadas disponibles")
		return
	}

	fsType, err := getFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error obteniendo tipo de sistema de archivos: %s\n", err)
		return
	}

	var authenticatedUser *User
	var userGroup *Group

	if fsType == estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		authenticatedUser, userGroup, err = authenticateUser(mountedPartition, user, pass)
	} else if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		authenticatedUser, userGroup, err = authenticateUserEXT3(mountedPartition, user, pass)
	}

	if err != nil {
		fmt.Printf("Error de autenticación: %s\n", err)
		return
	}

	// Iniciar sesión
	err = session.StartSession(
		authenticatedUser.UID,
		userGroup.GID,
		authenticatedUser.Username,
		userGroup.Name,
		id,
	)

	if err != nil {
		fmt.Printf("Error iniciando sesión: %s\n", err)
		return
	}

	// Mostrar mensaje de éxito
	fmt.Printf("\n=== SESIÓN INICIADA EXITOSAMENTE ===\n")
	fmt.Printf("Usuario: %s (UID: %d)\n", authenticatedUser.Username, authenticatedUser.UID)
	fmt.Printf("Grupo: %s (GID: %d)\n", userGroup.Name, userGroup.GID)
	fmt.Printf("Partición: %s (%s)\n", id, mountedPartition.Name)
	fmt.Printf("Disco: %s\n", mountedPartition.Path)
	fmt.Printf("Tipo: %s\n", mountedPartition.Type)
	fmt.Printf("Hora de inicio: %s\n", session.GetLoginTime())

	// Mostrar directorio actual (siempre empieza en /)
	fmt.Printf("Directorio actual: /\n")
	fmt.Printf("=====================================\n\n")

	fmt.Println("Sesión activa. Puede usar comandos del sistema de archivos.")
}

// User representa un usuario del sistema
type User struct {
	UID      int32
	Username string
	Password string
	GroupID  int32
}

// Group representa un grupo del sistema
type Group struct {
	GID  int32
	Name string
}

func authenticateUser(partition MountedPartition, username, password string) (*User, *Group, error) {

	// Leer el archivo users.txt de la partición
	usersContent, err := readUsersFile(partition)
	if err != nil {
		return nil, nil, fmt.Errorf("error accediendo al sistema de usuarios: %s", err)
	}

	// Parsear el contenido del archivo users.txt
	users, groups := parseUsersFile(usersContent)

	// Buscar el usuario (comparación case-insensitive)
	var foundUser *User
	for _, user := range users {
		if strings.EqualFold(user.Username, username) {
			if user.UID == 0 {
				return nil, nil, fmt.Errorf("el usuario '%s' está eliminado", username)
			}
			foundUser = &user
			break
		}
	}

	if foundUser == nil {
		fmt.Printf("DEBUG: Usuario '%s' no encontrado en la lista de %d usuarios\n",
			username, len(users))
		return nil, nil, fmt.Errorf("el usuario '%s' no existe", username)
	}

	if foundUser.Password != password {
		return nil, nil, fmt.Errorf("contraseña incorrecta para el usuario '%s'", username)
	}

	// Buscar el grupo del usuario
	var foundGroup *Group
	for _, group := range groups {
		if group.GID == foundUser.GroupID {
			if group.GID == 0 {
				return nil, nil, fmt.Errorf("el grupo del usuario está eliminado")
			}
			foundGroup = &group
			break
		}
	}

	if foundGroup == nil {
		return nil, nil, fmt.Errorf("grupo del usuario '%s' no encontrado", username)
	}

	return foundUser, foundGroup, nil
}

func authenticateUserEXT3(partition MountedPartition, username, password string) (*User, *Group, error) {

	// Leer el archivo users.txt de la partición
	usersContent, err := readUsersFileEXT3(partition)
	if err != nil {
		return nil, nil, fmt.Errorf("error accediendo al sistema de usuarios: %s", err)
	}

	users, groups := parseUsersFile(usersContent)

	var foundUser *User
	for _, user := range users {
		if strings.EqualFold(user.Username, username) {
			if user.UID == 0 {
				return nil, nil, fmt.Errorf("el usuario '%s' está eliminado", username)
			}
			foundUser = &user
			break
		}
	}

	if foundUser == nil {
		fmt.Printf("DEBUG: Usuario '%s' no encontrado en la lista de %d usuarios\n",
			username, len(users))
		return nil, nil, fmt.Errorf("el usuario '%s' no existe", username)
	}

	if foundUser.Password != password {
		return nil, nil, fmt.Errorf("contraseña incorrecta para el usuario '%s'", username)
	}

	// Buscar el grupo del usuario
	var foundGroup *Group
	for _, group := range groups {
		if group.GID == foundUser.GroupID {
			if group.GID == 0 {
				return nil, nil, fmt.Errorf("el grupo del usuario está eliminado")
			}
			foundGroup = &group
			break
		}
	}

	if foundGroup == nil {
		return nil, nil, fmt.Errorf("grupo del usuario '%s' no encontrado", username)
	}

	return foundUser, foundGroup, nil

}

// readUsersFile lee el contenido del archivo users.txt de la partición
func readUsersFile(partition MountedPartition) (string, error) {

	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return "", fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT2 de la partición
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return "", fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// Verificar que el sistema esté formateado
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return "", fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	// Buscar el archivo users.txt en el directorio raíz
	usersInodeNum, err := findFileInRootDirectory(file, ext2fs, "users.txt")
	if err != nil {
		return "", fmt.Errorf("archivo users.txt no encontrado: %s", err)
	}

	// Leer el inodo del archivo users.txt
	usersInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (usersInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return "", fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// Leer el contenido del archivo desde sus bloques
	content, err := readFileContentFromBlocks(file, ext2fs, usersInode)
	if err != nil {
		return "", fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	return content, nil
}

func readUsersFileEXT3(partition MountedPartition) (string, error) {

	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return "", fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return "", fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return "", fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	usersInodeNum, err := findFileInRootDirectoryEXT3(file, ext3fs, "users.txt")
	if err != nil {
		return "", fmt.Errorf("archivo users.txt no encontrado: %s", err)
	}

	usersInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (usersInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return "", fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	content, err := readFileContentFromBlocksEXT3(file, ext3fs, usersInode)
	if err != nil {
		return "", fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	return content, nil
}

// getPartitionStartOffset obtiene el offset de inicio de una partición montada
func getPartitionStartOffset(partition MountedPartition) int32 {
	// Abrir el disco para leer la información de la partición
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return 0
	}
	defer file.Close()

	if partition.Type == "Primaria" {
		// Leer MBR para encontrar la partición primaria
		var mbr structuresP2.MBR
		if err := utilitiesP2.ReadStruct(file, 0, &mbr); err != nil {
			return 0
		}

		// Buscar la partición por nombre
		for _, p := range mbr.MbrPartitions {
			if p.PartS > 0 && p.PartType == 'P' {
				partName := strings.TrimRight(string(p.PartName[:]), "\x00")
				if partName == partition.Name {
					return p.PartStart
				}
			}
		}
	} else if partition.Type == "Lógica" {
		// Para particiones lógicas, buscar en los EBRs
		start, _, err := findLogicalPartitionInfo(file, partition.Name)
		if err == nil {
			return start
		}
	}

	return 0
}

// findFileInRootDirectory busca un archivo en el directorio raíz
func findFileInRootDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, filename string) (int32, error) {
	// Leer el bloque del directorio raíz (bloque 0)
	rootBlock := &bloques.BloqueCarpeta{}
	rootBlockOffset := int64(ext2fs.SuperBlock.S_block_start)

	err := utilitiesP2.ReadStruct(file, rootBlockOffset, rootBlock)
	if err != nil {
		return -1, fmt.Errorf("error leyendo bloque del directorio raíz: %s", err)
	}

	//  CORREGIR: GetEntry retorna 3 valores
	for i := 0; i < 4; i++ { // BloqueCarpeta tiene 4 entradas
		entryName, inodeNum, exists := rootBlock.GetEntry(i) //  3 valores
		if exists && entryName == filename {
			return inodeNum, nil
		}
	}

	return -1, fmt.Errorf("archivo '%s' no encontrado en el directorio raíz", filename)
}

// readFileContent lee el contenido completo de un archivo desde sus bloques
func readFileContentFromBlocks(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo) (string, error) {
	var content strings.Builder
	blocks := inode.GetIblock()
	fileSize := inode.Is // Tamaño total del archivo
	bytesRead := int32(0)

	// Leer bloques directos
	for i := 0; i < 12 && bytesRead < fileSize; i++ {
		blockNum := blocks[i]
		if blockNum == -1 {
			break // No hay más bloques directos
		}

		// Leer el bloque de archivo
		fileBlock := &bloques.BloqueArchivo{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))

		err := utilitiesP2.ReadStruct(file, blockOffset, fileBlock)
		if err != nil {
			return "", fmt.Errorf("error leyendo bloque %d: %s", blockNum, err)
		}

		// Calcular cuántos bytes leer de este bloque
		bytesToRead := int32(64) // Tamaño de bloque
		if fileSize-bytesRead < bytesToRead {
			bytesToRead = fileSize - bytesRead
		}

		// Agregar el contenido del bloque
		blockContent := string(fileBlock.B_content[:bytesToRead])
		blockContent = strings.TrimRight(blockContent, "\x00")
		content.WriteString(blockContent)

		bytesRead += bytesToRead
	}

	// Leer bloques indirectos si es necesario
	if bytesRead < fileSize && blocks[12] != -1 {
		// Implementar lectura de bloques indirectos aquí si es necesario
		fmt.Printf("DEBUG: Se necesita leer bloques indirectos (%d bytes restantes)\n", fileSize-bytesRead)
	}

	fmt.Printf("DEBUG: Total bytes leídos: %d/%d\n", bytesRead, fileSize)
	return content.String(), nil
}

func parseUsersFile(content string) ([]User, []Group) {
	var users []User
	var groups []Group

	lines := strings.Split(content, "\n")

	// Primero procesar grupos para tener la lista completa
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
		id := int32(0)
		if len(parts[0]) > 0 {
			if parsed := parseInt32(parts[0]); parsed != -1 {
				id = parsed
			}
		}

		// Procesar solo grupos primero
		if parts[1] == "G" && len(parts) >= 3 {
			// Formato: ID,G,NombreGrupo
			if id == 0 {
				continue // Opcional: omitir grupos eliminados
			}
			group := Group{
				GID:  id,
				Name: strings.TrimSpace(parts[2]),
			}
			groups = append(groups, group)
		}
	}

	// Después procesar usuarios
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 5 {
			continue
		}

		// Parsear ID
		id := int32(0)
		if len(parts[0]) > 0 {
			if parsed := parseInt32(parts[0]); parsed != -1 {
				id = parsed
			}
		}

		// Procesar solo usuarios
		if parts[1] == "U" && len(parts) >= 5 {

			// CORRECIÓN AQUÍ: El formato es ID,U,GRUPO,USUARIO,CONTRASEÑA
			// pero lo estábamos interpretando como ID,U,USUARIO,GRUPO,CONTRASEÑA

			// FORMATO CORRECTO:
			groupName := strings.TrimSpace(parts[2])
			username := strings.TrimSpace(parts[3])
			password := strings.TrimSpace(parts[4])

			if id == 0 {
				continue // Usuario eliminado
			}

			// Buscar el ID del grupo por nombre
			groupID := int32(1) // Por defecto, grupo root
			groupFound := false
			for _, g := range groups {
				if strings.EqualFold(g.Name, groupName) {
					groupID = g.GID
					groupFound = true
					break
				}
			}

			if !groupFound {
				fmt.Printf("DEBUG:  ADVERTENCIA: Grupo '%s' no encontrado para usuario '%s'\n",
					groupName, username)
			}

			user := User{
				UID:      id,
				Username: username,
				Password: password,
				GroupID:  groupID,
			}
			users = append(users, user)

		}
	}

	return users, groups
}

// parseInt32 convierte string a int32, retorna -1 si hay error
func parseInt32(s string) int32 {
	if len(s) == 0 {
		return -1
	}

	// Conversión simple sin usar strconv para evitar dependencias
	result := int32(0)
	for _, char := range s {
		if char < '0' || char > '9' {
			return -1
		}
		result = result*10 + int32(char-'0')
	}
	return result
}

package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MKUSR implementa el comando para crear usuarios
func MKUSR(args []string) {
	// 1. Verificar que hay una sesión activa
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		fmt.Println("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	// 2. Obtener información de la sesión
	sessionInfo := session.GetCurrentSession()

	// 3. Verificar que el usuario es root
	if sessionInfo.Username != "root" {
		fmt.Printf("Error: Solo el usuario root puede crear usuarios\n")
		fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
		return
	}

	// 4. Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetros obligatorios
	username, userExists := params["user"]
	password, passExists := params["pass"]
	groupName, grpExists := params["grp"]

	if !userExists || username == "" {
		fmt.Println("Error: Debe especificar el nombre del usuario")
		fmt.Println("Uso: mkusr -user=<nombre> -pass=<contraseña> -grp=<grupo>")
		return
	}

	if !passExists || password == "" {
		fmt.Println("Error: Debe especificar la contraseña del usuario")
		fmt.Println("Uso: mkusr -user=<nombre> -pass=<contraseña> -grp=<grupo>")
		return
	}

	if !grpExists || groupName == "" {
		fmt.Println("Error: Debe especificar el grupo del usuario")
		fmt.Println("Uso: mkusr -user=<nombre> -pass=<contraseña> -grp=<grupo>")
		return
	}

	// Limpiar comillas si las tienen
	username = strings.Trim(username, "\"")
	password = strings.Trim(password, "\"")
	groupName = strings.Trim(groupName, "\"")

	// 5. Validaciones de longitud según especificación
	if len(username) > 10 {
		fmt.Printf("Error: El nombre del usuario no puede tener más de 10 caracteres (actual: %d)\n", len(username))
		return
	}

	if len(password) > 10 {
		fmt.Printf("Error: La contraseña no puede tener más de 10 caracteres (actual: %d)\n", len(password))
		return
	}

	if len(groupName) > 10 {
		fmt.Printf("Error: El nombre del grupo no puede tener más de 10 caracteres (actual: %d)\n", len(groupName))
		return
	}

	// 6. Validar que los campos no estén vacíos
	if strings.TrimSpace(username) == "" {
		fmt.Println("Error: El nombre del usuario no puede estar vacío")
		return
	}

	if strings.TrimSpace(password) == "" {
		fmt.Println("Error: La contraseña no puede estar vacía")
		return
	}

	if strings.TrimSpace(groupName) == "" {
		fmt.Println("Error: El nombre del grupo no puede estar vacío")
		return
	}

	// 7. Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	// 8. Crear el usuario
	err := createUser(mountedPartition, username, password, groupName)
	if err != nil {
		fmt.Printf("Error creando usuario: %s\n", err)
		return
	}

	fmt.Printf("Usuario '%s' creado exitosamente en el grupo '%s'\n", username, groupName)
}

func createUser(partition MountedPartition, username, password, groupName string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	fsType, err := detectFileSystemType(partition)
	if err != nil {
		return fmt.Errorf("error detectando sistema de archivos: %s", err)
	}

	if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		return createUserExt3(file, partition, username, password, groupName)
	} else {
		return createUserExt2(file, partition, username, password, groupName)
	}
}

func createUserExt3(file *os.File, partition MountedPartition, username, password, groupName string) error {

	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	usersInodeNum, err := findFileInRootDirectoryEXT3(file, ext3fs, "users.txt")
	if err != nil {
		return fmt.Errorf("archivo users.txt no encontrado: %s", err)
	}

	usersInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (usersInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	currentContent, err := readFileContentFromBlocksEXT3(file, ext3fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	if userExists(currentContent, username) {
		return fmt.Errorf("el usuario '%s' ya existe", username)
	}

	if !groupExistsAndActive(currentContent, groupName) {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_MKUSR, "/users.txt", "-")
		return fmt.Errorf("el grupo '%s' no existe", groupName)
	}

	nextUID := getNextUserID(currentContent)

	userEntry := fmt.Sprintf("%d,U,%s,%s,%s", nextUID, groupName, username, password)

	newContent := currentContent
	if !strings.HasSuffix(newContent, "\n") && newContent != "" {
		newContent += "\n"
	}
	newContent += userEntry + "\n"

	fmt.Printf("\nPREPARANDO CREACIÓN DE USUARIO:\n")
	fmt.Printf("Usuario: %s\n", username)
	fmt.Printf("Contraseña: %s\n", password)
	fmt.Printf("Grupo: %s\n", groupName)
	fmt.Printf("ID asignado: %d\n", nextUID)
	fmt.Printf("Entrada: %s\n", userEntry)

	newContentSize := int32(len([]byte(newContent)))
	fmt.Printf("Tamaño actual: %d bytes → Nuevo tamaño: %d bytes\n",
		int32(len([]byte(currentContent))), newContentSize)

	writeContentToFileUserEXT3(file, ext3fs, usersInode, newContent, usersInodeNum)

	err = writeContentToFileEXT3(file, ext3fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		fmt.Printf("\nERROR EN LA CREACIÓN DEL USUARIO:\n")
		fmt.Printf("Usuario que se intentó crear: %s\n", username)
		fmt.Printf("Motivo: %s\n", err)
		fmt.Printf("Sugerencia: El archivo users.txt necesita más bloques\n")
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKUSR, "/users.txt", "-")

	return nil
}

func createUserExt2(file *os.File, partition MountedPartition, username, password, groupName string) error {

	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	usersInodeNum, err := findFileInRootDirectory(file, ext2fs, "users.txt")
	if err != nil {
		return fmt.Errorf("archivo users.txt no encontrado: %s", err)
	}

	usersInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (usersInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	currentContent, err := readFileContentFromBlocks(file, ext2fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	if userExists(currentContent, username) {
		return fmt.Errorf("el usuario '%s' ya existe", username)
	}

	if !groupExistsAndActive(currentContent, groupName) {
		return fmt.Errorf("el grupo '%s' no existe", groupName)
	}

	nextUID := getNextUserID(currentContent)

	userEntry := fmt.Sprintf("%d,U,%s,%s,%s", nextUID, groupName, username, password)

	newContent := currentContent
	if !strings.HasSuffix(newContent, "\n") && newContent != "" {
		newContent += "\n"
	}
	newContent += userEntry + "\n"

	fmt.Printf("\nPREPARANDO CREACIÓN DE USUARIO:\n")
	fmt.Printf("Usuario: %s\n", username)
	fmt.Printf("Contraseña: %s\n", password)
	fmt.Printf("Grupo: %s\n", groupName)
	fmt.Printf("ID asignado: %d\n", nextUID)
	fmt.Printf("Entrada: %s\n", userEntry)

	newContentSize := int32(len([]byte(newContent)))
	fmt.Printf("Tamaño actual: %d bytes → Nuevo tamaño: %d bytes\n",
		int32(len([]byte(currentContent))), newContentSize)

	showFileSpaceInfo(usersInode, newContentSize)

	err = writeContentToFileUser(file, ext2fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		fmt.Printf("\nERROR EN LA CREACIÓN DEL USUARIO:\n")
		fmt.Printf("Usuario que se intentó crear: %s\n", username)
		fmt.Printf("Motivo: %s\n", err)
		fmt.Printf("Sugerencia: El archivo users.txt necesita más bloques\n")
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	return nil
}

// userExists verifica si un usuario ya existe en el contenido de users.txt
func userExists(content, username string) bool {
	lines := strings.Split(content, "\n")
	fmt.Printf("DEBUG: Verificando si usuario '%s' existe...\n", username)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parsear la línea: ID,TIPO,GRUPO,USUARIO,PASSWORD para usuarios
		parts := strings.Split(line, ",")
		//fmt.Printf("DEBUG: Línea %d: %s -> %d partes\n", lineNum+1, line, len(parts))

		if len(parts) >= 5 && parts[1] == "U" {
			// parts[0] = ID, parts[1] = TIPO (U), parts[2] = GRUPO, parts[3] = USUARIO, parts[4] = PASSWORD
			existingUser := strings.TrimSpace(parts[3])
			//fmt.Printf("DEBUG: Usuario encontrado: '%s' (buscando: '%s')\n", existingUser, username)

			if existingUser == username && parts[0] != "0" {
				fmt.Printf("DEBUG: ¡Usuario '%s' ya existe!\n", username)
				return true
			}
		}
	}

	fmt.Printf("DEBUG: Usuario '%s' no existe\n", username)
	return false
}

// groupExistsAndActive verifica si un grupo existe y está activo (ID != 0)
func groupExistsAndActive(content, groupName string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parsear la línea: ID,TIPO,NOMBRE para grupos
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			// parts[0] = ID, parts[1] = TIPO (G/U), parts[2] = NOMBRE
			if parts[1] == "G" && parts[2] == groupName && parts[0] != "0" {
				return true // Grupo existe y no está eliminado
			}
		}
	}
	return false
}

// getNextUserID obtiene el siguiente ID de usuario disponible
func getNextUserID(content string) int32 {
	maxUID := int32(1) // Empezar desde 2 (root es 1)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parsear todas las líneas para encontrar el ID más alto
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			// parts[0] = ID, parts[1] = TIPO (G/U)
			if id, err := strconv.Atoi(parts[0]); err == nil {
				if int32(id) > maxUID && parts[0] != "0" { // Ignorar IDs eliminados (0)
					maxUID = int32(id)
				}
			}
		}
	}

	return maxUID + 1
}

// writeContentToFileUser CORREGIDA - Con crecimiento automático
func writeContentToFileUser(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))

	fmt.Printf("DEBUG: Escribiendo %d bytes al archivo users.txt\n", contentSize)

	blocksNeeded := (contentSize + 63) / 64 // Redondear hacia arriba

	// Obtener bloques actuales
	blocks := inode.GetIblock()
	blocksUsed := int32(0)
	for i := 0; i < 12; i++ {
		if blocks[i] != -1 {
			blocksUsed++
		}
	}

	// Asignar bloques directos
	if blocksNeeded > blocksUsed {
		for i := int(blocksUsed); i < 12 && i < int(blocksNeeded); i++ {
			newBlockNum, err := findFreeBlock(file, ext2fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres disponibles: %s", err)
			}
			inode.SetIblock(i, newBlockNum)
			err = markBlockAsUsed(file, ext2fs, newBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque como usado: %s", err)
			}
			ext2fs.SuperBlock.S_free_blocks_count--
			fmt.Printf("Bloque %d asignado en posición %d\n", newBlockNum, i)
		}
	}

	// --- Indirecto simple ---
	var apuntador *bloques.BloqueApuntador
	if blocksNeeded > 12 {
		if inode.GetIblock()[12] != -1 {
			indirectBlockNum := inode.GetIblock()[12]
			apuntador = bloques.NewBloqueApuntador()
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (indirectBlockNum * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, apuntador)
			if err != nil {
				return fmt.Errorf("error leyendo bloque de apuntadores existente: %s", err)
			}
		} else {
			apuntador = bloques.NewBloqueApuntador()
			indirectBlockNum, err := findFreeBlock(file, ext2fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para bloque de apuntadores: %s", err)
			}
			inode.SetIblock(12, indirectBlockNum)
			err = markBlockAsUsed(file, ext2fs, indirectBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque de apuntadores como usado: %s", err)
			}
			ext2fs.SuperBlock.S_free_blocks_count--
		}

		// CORREGIDO: Asignar bloques faltantes en el apuntador simple
		for i := 12; i < int(blocksNeeded) && (i-12) < 16; i++ {
			if apuntador.B_pointers[i-12] == -1 {
				blockNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					return fmt.Errorf("no se pudo encontrar un bloque libre: %s", err)
				}
				apuntador.B_pointers[i-12] = blockNum
				err = markBlockAsUsed(file, ext2fs, blockNum)
				if err != nil {
					return fmt.Errorf("error marcando bloque como usado: %s", err)
				}
				ext2fs.SuperBlock.S_free_blocks_count--
			}
		}

		// Escribir el bloque de apuntadores actualizado
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (inode.GetIblock()[12] * ext2fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, apuntador)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque de apuntadores: %s", err)
		}
		fmt.Printf("[DEBUG] Bloque Apuntador Simple (bloque %d): %v\n", inode.GetIblock()[12], apuntador.B_pointers)
	}

	// --- Indirecto doble ---
	var dobleApuntador *bloques.BloqueApuntador // CORREGIDO: Declarar fuera del if
	if blocksNeeded > 28 {
		dobleApuntador = bloques.NewBloqueApuntador()
		if inode.GetIblock()[13] != -1 {
			dobleBlockNum := inode.GetIblock()[13]
			dobleOffset := int64(ext2fs.SuperBlock.S_block_start + (dobleBlockNum * ext2fs.SuperBlock.S_block_s))
			utilitiesP2.ReadStruct(file, dobleOffset, dobleApuntador)
		} else {
			dobleBlockNum, err := findFreeBlock(file, ext2fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para bloque de apuntadores doble: %s", err)
			}
			inode.SetIblock(13, dobleBlockNum)
			err = markBlockAsUsed(file, ext2fs, dobleBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque de apuntadores doble como usado: %s", err)
			}
			ext2fs.SuperBlock.S_free_blocks_count--
		}

		// CORREGIDO: Lógica de asignación de bloques dobles
		for i := int32(28); i < blocksNeeded && i < 284; i++ {
			hijoIndex := (i - 28) / 16
			hijoOffset := (i - 28) % 16

			var hijoNum int32
			var hijo *bloques.BloqueApuntador

			// Crear o leer el hijo
			if dobleApuntador.B_pointers[hijoIndex] == -1 {
				hijo = bloques.NewBloqueApuntador()
				hijoNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para bloque de apuntadores doble hijo: %s", err)
				}
				dobleApuntador.B_pointers[hijoIndex] = hijoNum
				err = markBlockAsUsed(file, ext2fs, hijoNum) // CORREGIDO: Marcar bloque hijo como usado
				if err != nil {
					return fmt.Errorf("error marcando bloque hijo como usado: %s", err)
				}
				ext2fs.SuperBlock.S_free_blocks_count--
			} else {
				hijoNum = dobleApuntador.B_pointers[hijoIndex]
				hijo = bloques.NewBloqueApuntador()
				hijoOffsetDisk := int64(ext2fs.SuperBlock.S_block_start + (hijoNum * ext2fs.SuperBlock.S_block_s))
				utilitiesP2.ReadStruct(file, hijoOffsetDisk, hijo)
			}

			// Asignar bloque de datos si no existe
			if hijo.B_pointers[hijoOffset] == -1 {
				blockNum, err := findFreeBlock(file, ext2fs)
				if err != nil {
					return fmt.Errorf("no se pudo encontrar un bloque libre: %s", err)
				}
				hijo.B_pointers[hijoOffset] = blockNum
				err = markBlockAsUsed(file, ext2fs, blockNum)
				if err != nil {
					return fmt.Errorf("error marcando bloque como usado: %s", err)
				}
				ext2fs.SuperBlock.S_free_blocks_count--
			}

			// Escribir el bloque hijo
			hijoOffsetDisk := int64(ext2fs.SuperBlock.S_block_start + (hijoNum * ext2fs.SuperBlock.S_block_s))
			utilitiesP2.WriteStruct(file, hijoOffsetDisk, hijo)
		}

		// Escribir el bloque doble principal
		dobleBlockNum := inode.GetIblock()[13]
		dobleOffset := int64(ext2fs.SuperBlock.S_block_start + (dobleBlockNum * ext2fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, dobleOffset, dobleApuntador)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque de apuntadores doble: %s", err)
		}
		fmt.Printf("[DEBUG] Bloque Apuntador Doble (bloque %d): %v\n", dobleBlockNum, dobleApuntador.B_pointers)
	}

	// CORREGIDO: Función getBlockNumber para manejar todos los tipos de bloques
	getBlockNumber := func(index int32) int32 {
		if index < 12 {
			return inode.GetIblock()[index]
		} else if index < 28 {
			if apuntador != nil {
				return apuntador.B_pointers[index-12]
			}
		} else if index < 284 {
			if dobleApuntador != nil {
				hijoIndex := (index - 28) / 16
				hijoOffset := (index - 28) % 16
				if dobleApuntador.B_pointers[hijoIndex] != -1 {
					// Leer el bloque hijo
					hijo := bloques.NewBloqueApuntador()
					hijoNum := dobleApuntador.B_pointers[hijoIndex]
					hijoOffsetDisk := int64(ext2fs.SuperBlock.S_block_start + (hijoNum * ext2fs.SuperBlock.S_block_s))
					utilitiesP2.ReadStruct(file, hijoOffsetDisk, hijo)
					return hijo.B_pointers[hijoOffset]
				}
			}
		}
		return -1
	}

	// ESCRIBIR CONTENIDO EN TODOS LOS BLOQUES NECESARIOS
	bytesWritten := int32(0)
	for blockIndex := 0; blockIndex < int(blocksNeeded); blockIndex++ {
		blockNum := getBlockNumber(int32(blockIndex)) // CORREGIDO: Usar la función helper
		if blockNum == -1 {
			return fmt.Errorf("bloque %d no asignado correctamente", blockIndex)
		}

		remainingBytes := contentSize - bytesWritten
		bytesToWrite := int32(64)
		if remainingBytes < bytesToWrite {
			bytesToWrite = remainingBytes
		}

		fileBlock := &bloques.BloqueArchivo{}
		for i := 0; i < 64; i++ {
			fileBlock.B_content[i] = 0
		}
		if bytesToWrite > 0 {
			copy(fileBlock.B_content[:bytesToWrite], contentBytes[bytesWritten:bytesWritten+bytesToWrite])
		}

		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque %d: %s", blockNum, err)
		}
		bytesWritten += bytesToWrite
		fmt.Printf("DEBUG: Bloque %d escrito: %d bytes (total: %d)\n", blockIndex, bytesToWrite, bytesWritten)
	}

	// LIMPIAR BLOQUES NO USADOS (si el archivo se redujo)
	for i := int(blocksNeeded); i < 12; i++ {
		if inode.GetIblock()[i] != -1 {
			oldBlockNum := inode.GetIblock()[i]
			emptyBlock := &bloques.BloqueArchivo{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (oldBlockNum * ext2fs.SuperBlock.S_block_s))
			utilitiesP2.WriteStruct(file, blockOffset, emptyBlock)
			markBlockAsFree(file, ext2fs, oldBlockNum)
			inode.SetIblock(i, -1)
			ext2fs.SuperBlock.S_free_blocks_count++
			fmt.Printf("🗑️  Bloque %d liberado de posición %d\n", oldBlockNum, i)
		}
	}

	// Actualizar el superbloque
	err := utilitiesP2.WriteStruct(file, int64(ext2fs.StartOffset), ext2fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	// ACTUALIZAR EL INODO
	inode.SetSize(contentSize)
	inode.UpdateModificationTime()
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (inodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo: %s", err)
	}

	return nil
}

// showFileSpaceInfo muestra información detallada del espacio del archivo
func showFileSpaceInfo(inode *estructuraArchivoCarpeta.Inodo, contentSize int32) {
	blocks := inode.GetIblock()

	blocksAvailable := int32(0)
	for i := 0; i < 16; i++ {
		if blocks[i] != -1 {
			blocksAvailable++
		} else {
			break
		}
	}

	totalSpace := blocksAvailable * 64
	usedSpace := contentSize
	freeSpace := totalSpace - usedSpace
	usagePercentage := float64(usedSpace) / float64(totalSpace) * 100

	fmt.Printf("\nESTADÍSTICAS DEL ARCHIVO users.txt:\n")
	fmt.Printf("Bloques asignados: %d (máximo 16)\n", blocksAvailable)
	fmt.Printf("Espacio total: %d bytes (%d bloques × 64 bytes)\n", totalSpace, blocksAvailable)
	fmt.Printf("Espacio usado: %d bytes (%.1f%%)\n", usedSpace, usagePercentage)
	fmt.Printf("Espacio libre: %d bytes\n", freeSpace)

	if freeSpace <= 0 {
		fmt.Printf(" ¡ARCHIVO LLENO! No se pueden agregar más usuarios\n")
	} else if freeSpace < 50 { // Menos de 50 bytes libres
		fmt.Printf(" ¡ESPACIO CRÍTICO! Solo quedan %d bytes libres\n", freeSpace)
	}
	fmt.Printf("=====================================\n")
}

// Marcar bloque como usado
func markBlockAsUsed(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32) error {
	return updateBlockBitmap(file, ext2fs, blockNum, true)
}

// Marcar bloque como libre
func markBlockAsFree(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32) error {
	return updateBlockBitmap(file, ext2fs, blockNum, false)
}

// Actualizar bitmap de bloques
func updateBlockBitmap(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, used bool) error {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	// Leer el byte específico del bitmap
	_, err := file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
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
	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return err
	}

	_, err = file.Write(currentByte[:])
	return err
}

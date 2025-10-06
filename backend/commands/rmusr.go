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
	"strings"
)

// RMUSR implementa el comando para eliminar usuarios
func RMUSR(args []string) {
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
		fmt.Printf("Error: Solo el usuario root puede eliminar usuarios\n")
		fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
		return
	}

	// 4. Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	username, exists := params["user"]
	if !exists || username == "" {
		fmt.Println("Error: Debe especificar el nombre del usuario")
		fmt.Println("Uso: rmusr -user=<nombre_usuario>")
		return
	}

	// Limpiar comillas si las tiene
	username = strings.Trim(username, "\"")

	// 5. Validar nombre del usuario
	if strings.TrimSpace(username) == "" {
		fmt.Println("Error: El nombre del usuario no puede estar vacío")
		return
	}

	// 6. Validar que no se intente eliminar el usuario root
	if username == "root" {
		fmt.Println("Error: No se puede eliminar el usuario 'root'")
		return
	}

	// 7. Obtener la partición montada
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
		err = removeUser(mountedPartition, username)
	} else if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		err = removeUserEXT3(mountedPartition, username)
	} else {
		fmt.Printf("Error: El sistema de archivos '%s' no es compatible con este comando\n", fsType)
		return
	}

	if err != nil {
		fmt.Printf("Error eliminando usuario: %s\n", err)
		return
	}

	fmt.Printf("Usuario '%s' eliminado exitosamente\n", username)
}

// removeUser elimina un usuario del archivo users.txt marcándolo con ID 0
func removeUser(partition MountedPartition, username string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()
	// Lógica para cambiar el grupo del usuario en la partición montada
	// Leer el sistema EXT2 de la partición
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// Verificar que el sistema esté formateado
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	// Buscar el archivo users.txt en el directorio raíz
	usersInodeNum, err := findFileInRootDirectory(file, ext2fs, "users.txt")
	if err != nil {
		return fmt.Errorf("archivo users.txt no encontrado: %s", err)
	}

	// Leer el inodo del archivo users.txt
	usersInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (usersInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// Leer el contenido actual del archivo users.txt
	currentContent, err := readFileContentFromBlocks(file, ext2fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	// Eliminar el usuario (marcarlo con ID 0)
	newContent, userFound := markUserAsDeleted(currentContent, username)
	if !userFound {
		return fmt.Errorf("el usuario '%s' no existe", username)
	}

	// Escribir el contenido actualizado de vuelta al archivo
	err = writeContentToFileRMUSR(file, ext2fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	return nil
}

func removeUserEXT3(partition MountedPartition, username string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	// Verificar que el sistema esté formateado
	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	// Buscar el archivo users.txt en el directorio raíz
	usersInodeNum, err := findFileInRootDirectoryEXT3(file, ext3fs, "users.txt")
	if err != nil {
		return fmt.Errorf("archivo users.txt no encontrado: %s", err)
	}

	// Leer el inodo del archivo users.txt
	usersInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (usersInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// Leer el contenido actual del archivo users.txt
	currentContent, err := readFileContentFromBlocksEXT3(file, ext3fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	// Eliminar el usuario (marcarlo con ID 0)
	newContent, userFound := markUserAsDeleted(currentContent, username)
	if !userFound {
		return fmt.Errorf("el usuario '%s' no existe", username)
	}

	// Escribir el contenido actualizado de vuelta al archivo
	err = writeContentToFileRMUSREXT3(file, ext3fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_RMUSR, "users.txt", fmt.Sprintf("Usuario '%s' eliminado por '%s'", username, session.GetCurrentSession().Username))
	if err != nil {
		return fmt.Errorf("error añadiendo entrada al journal: %s", err)
	}

	return nil
}

// markUserAsDeleted marca un usuario como eliminado cambiando su ID a 0
func markUserAsDeleted(content, username string) (string, bool) {
	lines := strings.Split(content, "\n")
	var newLines []string
	userFound := false

	for _, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		if line == "" {
			newLines = append(newLines, originalLine)
			continue
		}

		// Parsear la línea: ID,TIPO,GRUPO,USUARIO,PASSWORD para usuarios
		parts := strings.Split(line, ",")

		if len(parts) >= 5 && parts[1] == "U" {
			userID := parts[0]
			user := parts[3]

			// Verificar si este es el usuario que buscamos
			if parts[1] == "U" && strings.EqualFold(user, username) && userID != "0" {
				fmt.Printf("DEBUG: ¡Usuario '%s' encontrado! Marcando como eliminado\n", username)
				// Marcar como eliminado con ID 0
				parts[0] = "0"
				newLine := strings.Join(parts, ",")
				newLines = append(newLines, newLine)
				userFound = true
			} else {
				if parts[1] == "U" && strings.EqualFold(user, username) {
					if userID == "0" {
						fmt.Printf("DEBUG: El usuario '%s' ya está eliminado (ID=0)\n", username)
					}
				}
				newLines = append(newLines, originalLine)
			}
		} else {
			// Para grupos o líneas con formato diferente
			newLines = append(newLines, originalLine)
		}
	}

	if userFound {
		fmt.Printf("DEBUG: Usuario '%s' encontrado y marcado como eliminado\n", username)
	} else {
		fmt.Printf("DEBUG: Usuario '%s' NO encontrado en users.txt\n", username)
	}

	return strings.Join(newLines, "\n"), userFound
}

// writeContentToFileRMUSR escribe contenido actualizado al archivo users.txt (específico para RMUSR)
func writeContentToFileRMUSR(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))

	// Obtener los bloques actuales del archivo
	blocks := inode.GetIblock()

	// Escribir el contenido en los bloques existentes
	bytesWritten := int32(0)
	blockIndex := 0

	for bytesWritten < contentSize && blockIndex < 16 {
		blockNum := blocks[blockIndex]
		if blockNum == -1 {
			break
		}

		// Calcular cuántos bytes escribir en este bloque
		remainingBytes := contentSize - bytesWritten
		bytesToWrite := int32(64)
		if remainingBytes < bytesToWrite {
			bytesToWrite = remainingBytes
		}

		// Crear el bloque de archivo
		fileBlock := &bloques.BloqueArchivo{}

		// Copiar el contenido al bloque
		copy(fileBlock.B_content[:], contentBytes[bytesWritten:bytesWritten+bytesToWrite])

		// Llenar el resto con zeros si es necesario
		if bytesToWrite < 64 {
			for i := bytesToWrite; i < 64; i++ {
				fileBlock.B_content[i] = 0
			}
		}

		// Escribir el bloque al disco
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque %d: %s", blockNum, err)
		}

		bytesWritten += bytesToWrite
		blockIndex++
	}

	// Actualizar el tamaño del archivo en el inodo
	inode.SetSize(contentSize)
	inode.UpdateModificationTime()

	// Escribir el inodo actualizado
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (inodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo: %s", err)
	}

	return nil
}

func writeContentToFileRMUSREXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))

	// Obtener los bloques actuales del archivo
	blocks := inode.GetIblock()

	// Escribir el contenido en los bloques existentes
	bytesWritten := int32(0)
	blockIndex := 0

	for bytesWritten < contentSize && blockIndex < 15 {
		blockNum := blocks[blockIndex]
		if blockNum == -1 {
			break
		}

		// Calcular cuántos bytes escribir en este bloque
		remainingBytes := contentSize - bytesWritten
		bytesToWrite := int32(64)
		if remainingBytes < bytesToWrite {
			bytesToWrite = remainingBytes
		}

		// Crear el bloque de archivo
		fileBlock := &bloques.BloqueArchivo{}

		// Copiar el contenido al bloque
		copy(fileBlock.B_content[:], contentBytes[bytesWritten:bytesWritten+bytesToWrite])

		// Llenar el resto con zeros si es necesario
		if bytesToWrite < 64 {
			for i := bytesToWrite; i < 64; i++ {
				fileBlock.B_content[i] = 0
			}
		}

		// Escribir el bloque al disco
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque %d: %s", blockNum, err)
		}

		bytesWritten += bytesToWrite
		blockIndex++
	}

	// Actualizar el tamaño del archivo en el inodo
	inode.SetSize(contentSize)
	inode.UpdateModificationTime()

	// Escribir el inodo actualizado
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (inodeNum * ext3fs.SuperBlock.S_inode_s))
	err := utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo: %s", err)
	}

	return nil
}

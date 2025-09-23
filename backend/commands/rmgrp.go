package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
)

// RMGRP implementa el comando para eliminar grupos
func RMGRP(args []string) {
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
		fmt.Printf("Error: Solo el usuario root puede eliminar grupos\n")
		fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
		return
	}

	// 4. Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	groupName, exists := params["name"]
	if !exists || groupName == "" {
		fmt.Println("Error: Debe especificar el nombre del grupo")
		fmt.Println("Uso: rmgrp -name=<nombre_grupo>")
		return
	}

	// Limpiar comillas si las tiene
	groupName = strings.Trim(groupName, "\"")

	// 5. Validar nombre del grupo
	if strings.TrimSpace(groupName) == "" {
		fmt.Println("Error: El nombre del grupo no puede estar vacío")
		return
	}

	// 6. Validar que no se intente eliminar el grupo root
	if groupName == "root" {
		fmt.Println("Error: No se puede eliminar el grupo 'root'")
		return
	}

	// 7. Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	// 8. Eliminar el grupo
	err := removeGroup(mountedPartition, groupName)
	if err != nil {
		fmt.Printf("Error eliminando grupo: %s\n", err)
		return
	}

	fmt.Printf("Grupo '%s' eliminado exitosamente\n", groupName)
}

// removeGroup elimina un grupo del archivo users.txt marcándolo con ID 0
func removeGroup(partition MountedPartition, groupName string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

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

	//  Eliminar el grupo (marcarlo con ID 0)
	newContent, groupFound := markGroupAsDeleted(currentContent, groupName)
	if !groupFound {
		return fmt.Errorf("el grupo '%s' no existe", groupName)
	}

	//  Verificar si hay usuarios que pertenecen a este grupo
	if hasUsersInGroup(currentContent, groupName) {
		return fmt.Errorf("no se puede eliminar el grupo '%s': tiene usuarios asignados", groupName)
	}

	//  Escribir el contenido actualizado de vuelta al archivo
	err = writeContentToFileRMGRP(file, ext2fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	return nil
}

// markGroupAsDeleted marca un grupo como eliminado cambiando su ID a 0
func markGroupAsDeleted(content, groupName string) (string, bool) {
	lines := strings.Split(content, "\n")
	var newLines []string
	groupFound := false

	for _, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		if line == "" {
			newLines = append(newLines, originalLine)
			continue
		}

		// Parsear la línea: ID,TIPO,NOMBRE[,USUARIO,PASSWORD]
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			// parts[0] = ID, parts[1] = TIPO (G/U), parts[2] = NOMBRE
			if parts[1] == "G" && parts[2] == groupName {
				//Marcar como eliminado con ID 0
				parts[0] = "0"
				newLine := strings.Join(parts, ",")
				newLines = append(newLines, newLine)
				groupFound = true
			} else {
				newLines = append(newLines, originalLine)
			}
		} else {
			newLines = append(newLines, originalLine)
		}
	}

	return strings.Join(newLines, "\n"), groupFound
}

// hasUsersInGroup verifica si hay usuarios que pertenecen al grupo especificado
func hasUsersInGroup(content, groupName string) bool {
	// Primero, obtener el ID del grupo
	groupID := getGroupID(content, groupName)
	if groupID == "" {
		return false
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parsear la línea: ID,TIPO,NOMBRE[,USUARIO,PASSWORD]
		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			// parts[0] = ID, parts[1] = TIPO (G/U), parts[2] = USUARIO, parts[3] = GRUPO, parts[4] = PASSWORD
			if parts[1] == "U" && parts[3] == groupName {
				return true // Hay un usuario que pertenece a este grupo
			}
		}
	}
	return false
}

// getGroupID obtiene el ID de un grupo dado su nombre
func getGroupID(content, groupName string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parsear la línea: ID,TIPO,NOMBRE[,USUARIO,PASSWORD]
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			// parts[0] = ID, parts[1] = TIPO (G/U), parts[2] = NOMBRE
			if parts[1] == "G" && parts[2] == groupName {
				return parts[0]
			}
		}
	}
	return ""
}

// writeContentToFileRMGRP escribe contenido actualizado al archivo users.txt (específico para RMGRP)
func writeContentToFileRMGRP(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	// Reutilizar la función de writeContentToFile de MKGRP
	// Para evitar duplicación, se puede usar la misma lógica

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

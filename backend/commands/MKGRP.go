package commands

import (
	bloques "MIA_2S2025_P1_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	session "MIA_2S2025_P1_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"MIA_2S2025_P1_202307820/utilities"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MKGRP implementa el comando para crear grupos
func MKGRP(args []string) {
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
		fmt.Printf("Error: Solo el usuario root puede crear grupos\n")
		fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
		return
	}

	// 4. Parsear parámetros
	params := utilities.ParseParameters(args)

	groupName, exists := params["name"]
	if !exists || groupName == "" {
		fmt.Println("Error: Debe especificar el nombre del grupo")
		fmt.Println("Uso: mkgrp -name=<nombre_grupo>")
		return
	}

	// Limpiar comillas si las tiene
	groupName = strings.Trim(groupName, "\"")

	// 5. Validar nombre del grupo
	if strings.TrimSpace(groupName) == "" {
		fmt.Println("Error: El nombre del grupo no puede estar vacío")
		return
	}

	// 6. Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	// 7. Crear el grupo
	err := createGroup(mountedPartition, groupName)
	if err != nil {
		fmt.Printf("Error creando grupo: %s\n", err)
		return
	}

	fmt.Printf("Grupo '%s' creado exitosamente\n", groupName)
}

// createGroup crea un grupo en el archivo users.txt
func createGroup(partition MountedPartition, groupName string) error {
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
	err = utilities.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// Leer el contenido actual del archivo users.txt
	currentContent, err := readFileContentFromBlocks(file, ext2fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	// Verificar que el grupo no existe ya
	if groupExists(currentContent, groupName) {
		return fmt.Errorf("el grupo '%s' ya existe", groupName)
	}

	// Obtener el siguiente ID de grupo disponible
	nextGID := getNextGroupID(currentContent)

	// Crear la entrada del nuevo grupo
	groupEntry := fmt.Sprintf("%d,G,%s", nextGID, groupName)

	// Agregar la nueva línea al contenido
	newContent := currentContent
	if !strings.HasSuffix(newContent, "\n") && newContent != "" {
		newContent += "\n"
	}
	newContent += groupEntry + "\n"

	// Escribir el contenido actualizado de vuelta al archivo
	err = writeContentToFile(file, ext2fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	return nil
}

func groupExists(content, groupName string) bool {
	lines := strings.Split(content, "\n")
	fmt.Printf("Verificando si el grupo '%s' existe en users.txt...\n", groupName)

	targetGroupName := strings.ToLower(strings.TrimSpace(groupName))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 3 && parts[1] == "G" {
			existingGroupName := strings.ToLower(strings.TrimSpace(parts[2]))
			fmt.Printf("  Comparando con grupo existente: ID=%s, Nombre='%s'\n", parts[0], parts[2])

			if existingGroupName == targetGroupName {
				fmt.Printf("  ¡Grupo '%s' ya existe con ID %s!\n", groupName, parts[0])
				return true
			}
		}
	}
	fmt.Printf("  Grupo '%s' no existe en el sistema\n", groupName)
	return false
}

// getNextGroupID obtiene el siguiente ID de grupo disponible
func getNextGroupID(content string) int32 {
	maxGID := int32(1) // Empezar desde 2 (root es 1)

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
			if parts[1] == "G" { // Solo grupos
				if id, err := strconv.Atoi(parts[0]); err == nil {
					if int32(id) > maxGID {
						maxGID = int32(id)
					}
				}
			}
		}
	}

	return maxGID + 1
}

// writeContentToFile escribe contenido actualizado al archivo users.txt
func writeContentToFile(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))

	// Obtener los bloques actuales del archivo
	blocks := inode.GetIblock()

	// Calcular cuántos bloques necesitamos y verificar límites
	blocksNeeded := (contentSize + 63) / 64 // Redondear hacia arriba (64 bytes por bloque)

	// USAR la variable para verificar límites
	if blocksNeeded > 16 {
		return fmt.Errorf("el contenido es demasiado grande: necesita %d bloques, máximo 16", blocksNeeded)
	}

	// Escribir el contenido en los bloques existentes
	bytesWritten := int32(0)
	blockIndex := 0

	for bytesWritten < contentSize && blockIndex < 16 {
		blockNum := blocks[blockIndex]
		if blockNum == -1 {
			// Necesitamos asignar un nuevo bloque
			// Por simplicidad, usar el primer bloque disponible del archivo existente
			break
		}

		// Calcular cuántos bytes escribir en este bloque
		remainingBytes := contentSize - bytesWritten
		bytesToWrite := int32(64) // Tamaño del bloque
		if remainingBytes < bytesToWrite {
			bytesToWrite = remainingBytes
		}

		// Crear el bloque de archivo
		fileBlock := &bloques.BloqueArchivo{}

		// Copiar el contenido al bloque
		copy(fileBlock.B_content[:], contentBytes[bytesWritten:bytesWritten+bytesToWrite])

		// Si no llenamos el bloque completo, llenar el resto con zeros
		if bytesToWrite < 64 {
			for i := bytesToWrite; i < 64; i++ {
				fileBlock.B_content[i] = 0
			}
		}

		// Escribir el bloque al disco
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err := utilities.WriteStruct(file, blockOffset, fileBlock)
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
	err := utilities.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo: %s", err)
	}

	return nil
}

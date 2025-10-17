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
	params := utilitiesP2.ParseParameters(args)

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

	// Detectar tipo de sistema de archivos
	fsType, err := detectFileSystemType(partition)
	if err != nil {
		return fmt.Errorf("error detectando tipo de sistema de archivos: %s", err)
	}

	fmt.Printf("Sistema de archivos detectado: %s\n", getFileSystemTypeName(fsType))

	// Procesar según el tipo de sistema de archivos
	if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		return createGroupEXT3(file, partition, groupName)
	} else {
		return createGroupEXT2(file, partition, groupName)
	}
}

// createGroupEXT3 crea un grupo en EXT3 con journaling
func createGroupEXT3(file *os.File, partition MountedPartition, groupName string) error {
	// Leer el sistema EXT3
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	// Verificar que el sistema esté formateado correctamente
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
	err = writeContentToFileEXT3(file, ext3fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	// REGISTRO EN JOURNAL: Operación completada exitosamente
	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKGRP, "/users.txt", "-")

	//fmt.Printf("Grupo '%s' creado en EXT3 con journaling (GID: %d)\n", groupName, nextGID)
	return nil
}

// createGroupEXT2 crea un grupo en EXT2 (lógica original)
func createGroupEXT2(file *os.File, partition MountedPartition, groupName string) error {
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

	fmt.Printf("Grupo '%s' creado en EXT2 (GID: %d)\n", groupName, nextGID)
	return nil
}

// === FUNCIONES AUXILIARES PARA EXT3 ===

// findFileInRootDirectoryEXT3 busca un archivo en el directorio raíz en EXT3
func findFileInRootDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, filename string) (int32, error) {
	// Leer el bloque del directorio raíz (bloque 0)
	rootBlock := &bloques.BloqueCarpeta{}
	rootBlockOffset := int64(ext3fs.SuperBlock.S_block_start)

	err := utilitiesP2.ReadStruct(file, rootBlockOffset, rootBlock)
	if err != nil {
		return -1, fmt.Errorf("error leyendo bloque del directorio raíz: %s", err)
	}

	// Buscar el archivo en las entradas del bloque
	for i := 0; i < 4; i++ {
		entryName, inodeNum, exists := rootBlock.GetEntry(i)
		if exists && entryName == filename {
			return inodeNum, nil
		}
	}

	return -1, fmt.Errorf("archivo '%s' no encontrado en el directorio raíz", filename)
}

// readFileContentFromBlocksEXT3 - VERSION CON MAS DEBUG
func readFileContentFromBlocksEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *estructuraArchivoCarpeta.Inodo) (string, error) {
	fileSize := inode.Is

	if fileSize <= 0 {
		return "", nil
	}

	fmt.Printf("\n=== DEBUG LECTURA EXT3 ===\n")
	fmt.Printf("Tamaño del archivo según inodo: %d bytes\n", fileSize)

	contentBuffer := make([]byte, fileSize)
	bytesRead := int32(0)

	blocks := inode.GetIblock()
	//fmt.Printf("Bloques del inodo: %v\n", blocks[:4])

	// Leer bloques directos (0-11)
	for i := 0; i < 12 && bytesRead < fileSize; i++ {
		blockNum := blocks[i]
		if blockNum == -1 {
			break
		}

		fileBlock := &bloques.BloqueArchivo{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))

		err := utilitiesP2.ReadStruct(file, blockOffset, fileBlock)
		if err != nil {
			return "", fmt.Errorf("error leyendo bloque %d: %s", blockNum, err)
		}

		bytesToRead := int32(64)
		if fileSize-bytesRead < bytesToRead {
			bytesToRead = fileSize - bytesRead
		}

		copy(contentBuffer[bytesRead:bytesRead+bytesToRead], fileBlock.B_content[:bytesToRead])
		/*
			// DEBUG: Mostrar qué se leyó
			blockContent := string(fileBlock.B_content[:bytesToRead])
			fmt.Printf("\n--- DEBUG BLOQUE %d (num: %d) ---\n", i, blockNum)
			fmt.Printf("Bytes leídos: %d\n", bytesToRead)
			fmt.Printf("Contenido del bloque:\n%s\n", blockContent)
			fmt.Printf("-------------------------------\n")
		*/
		bytesRead += bytesToRead
		//fmt.Printf("DEBUG: Bloque directo %d leído - %d bytes (total: %d/%d)\n", i, bytesToRead, bytesRead, fileSize)
	}

	// Leer bloque indirecto simple (índice 12) si es necesario
	if bytesRead < fileSize && blocks[12] != -1 {
		fmt.Printf("DEBUG: Leyendo bloque indirecto simple: %d\n", blocks[12])

		apuntadorBlock := &bloques.BloqueApuntador{}
		apuntadorOffset := int64(ext3fs.SuperBlock.S_block_start + (blocks[12] * ext3fs.SuperBlock.S_block_s))

		err := utilitiesP2.ReadStruct(file, apuntadorOffset, apuntadorBlock)
		if err != nil {
			return "", fmt.Errorf("error leyendo bloque de apuntadores: %s", err)
		}

		for i := 0; i < 16 && bytesRead < fileSize; i++ {
			blockNum := apuntadorBlock.B_pointers[i]
			if blockNum == -1 {
				continue
			}

			fileBlock := &bloques.BloqueArchivo{}
			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))

			err := utilitiesP2.ReadStruct(file, blockOffset, fileBlock)
			if err != nil {
				fmt.Printf("DEBUG: Error leyendo bloque indirecto %d: %s\n", blockNum, err)
				continue
			}

			bytesToRead := int32(64)
			if fileSize-bytesRead < bytesToRead {
				bytesToRead = fileSize - bytesRead
			}

			copy(contentBuffer[bytesRead:bytesRead+bytesToRead], fileBlock.B_content[:bytesToRead])
			bytesRead += bytesToRead

			fmt.Printf("DEBUG: Bloque indirecto %d (%d) leído - %d bytes (total: %d/%d)\n",
				i, blockNum, bytesToRead, bytesRead, fileSize)
		}
	}

	result := string(contentBuffer)

	fmt.Printf("\n=== CONTENIDO COMPLETO LEÍDO ===\n")
	fmt.Printf("Total bytes leídos: %d/%d\n", bytesRead, fileSize)
	fmt.Printf("Contenido:\n%s\n", result)
	fmt.Printf("================================\n\n")

	return result, nil
}

// writeContentToFileEXT3 escribe contenido actualizado al archivo users.txt en EXT3 - VERSIÓN CORREGIDA
func writeContentToFileEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))
	/*
		fmt.Printf("\n=== DEBUG ESCRITURA EXT3 (MKGRP) ===\n")
		fmt.Printf("Contenido a escribir (%d bytes):\n%s\n", contentSize, content)
		fmt.Printf("====================================\n\n")
	*/
	// Calcular bloques necesarios
	blocksNeeded := (contentSize + 63) / 64 // Redondear hacia arriba

	// Obtener bloques actuales
	blocks := inode.GetIblock()
	directBlocksUsed := int32(0)
	for i := 0; i < 12; i++ {
		if blocks[i] != -1 {
			directBlocksUsed++
		}
	}

	//fmt.Printf("DEBUG: Bloques necesarios: %d, Bloques directos actuales: %d\n", blocksNeeded, directBlocksUsed)
	//fmt.Printf("DEBUG: Bloques actuales del inodo: %v\n", blocks[:4])

	// === ASIGNAR BLOQUES DIRECTOS ADICIONALES SI SON NECESARIOS ===
	for i := int(directBlocksUsed); i < 12 && i < int(blocksNeeded); i++ {
		newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil {
			return fmt.Errorf("no hay bloques libres disponibles: %s", err)
		}

		// VERIFICAR QUE EL BLOQUE NO ESTÉ YA ASIGNADO
		isAlreadyUsed := false
		for j := 0; j < i; j++ {
			if inode.GetIblock()[j] == newBlockNum {
				isAlreadyUsed = true
				break
			}
		}

		if isAlreadyUsed {
			fmt.Printf("⚠️  ADVERTENCIA: Bloque %d ya está asignado, buscando otro...\n", newBlockNum)
			// Marcar como usado para que findFreeBlockEXT3 no lo vuelva a encontrar
			err = markBlockAsUsedEXT3(file, ext3fs, newBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque temporal como usado: %s", err)
			}
			i-- // Reintentar con el mismo índice
			continue
		}

		inode.SetIblock(i, newBlockNum)
		err = markBlockAsUsedEXT3(file, ext3fs, newBlockNum)
		if err != nil {
			return fmt.Errorf("error marcando bloque como usado: %s", err)
		}
		ext3fs.SuperBlock.S_free_blocks_count--
		//fmt.Printf("✅ Bloque adicional %d asignado en posición %d del inodo %d\n", newBlockNum, i, inodeNum)

	}
	/*
		// Imprimir estado final de bloques asignados
		iblockArray := inode.GetIblock()
		iblockSlice := iblockArray[:]
		fmt.Printf("DEBUG: Bloques finales del inodo: %v\n", iblockSlice[:4])
	*/
	// === MANEJAR BLOQUE INDIRECTO SIMPLE (índice 12) SI ES NECESARIO ===
	var apuntador *bloques.BloqueApuntador
	if blocksNeeded > 12 {
		if inode.GetIblock()[12] != -1 {
			indirectBlockNum := inode.GetIblock()[12]
			apuntador = bloques.NewBloqueApuntador()
			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (indirectBlockNum * ext3fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, apuntador)
			if err != nil {
				return fmt.Errorf("error leyendo bloque de apuntadores existente: %s", err)
			}
			fmt.Printf("DEBUG: Bloque indirecto existente: %d\n", indirectBlockNum)
		} else {
			apuntador = bloques.NewBloqueApuntador()
			indirectBlockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para bloque de apuntadores: %s", err)
			}
			inode.SetIblock(12, indirectBlockNum)
			err = markBlockAsUsedEXT3(file, ext3fs, indirectBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque de apuntadores como usado: %s", err)
			}
			ext3fs.SuperBlock.S_free_blocks_count--
			fmt.Printf("DEBUG: Nuevo bloque indirecto asignado: %d\n", indirectBlockNum)

		}

		for i := 12; i < int(blocksNeeded) && (i-12) < 16; i++ {
			if apuntador.B_pointers[i-12] == -1 {
				blockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no se pudo encontrar un bloque libre: %s", err)
				}
				apuntador.B_pointers[i-12] = blockNum
				err = markBlockAsUsedEXT3(file, ext3fs, blockNum)
				if err != nil {
					return fmt.Errorf("error marcando bloque como usado: %s", err)
				}
				ext3fs.SuperBlock.S_free_blocks_count--
				fmt.Printf("DEBUG: Bloque indirecto %d asignado en apuntador[%d]\n", blockNum, i-12)
			}
		}

		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (inode.GetIblock()[12] * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, apuntador)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque de apuntadores: %s", err)
		}
	}

	// === FUNCIÓN HELPER PARA OBTENER NÚMERO DE BLOQUE ===
	getBlockNumber := func(index int32) int32 {
		if index < 12 {
			blockNum := inode.GetIblock()[index]
			//fmt.Printf("DEBUG: getBlockNumber(%d) = %d (directo)\n", index, blockNum)
			return blockNum
		} else if index < 28 && apuntador != nil {
			blockNum := apuntador.B_pointers[index-12]
			//fmt.Printf("DEBUG: getBlockNumber(%d) = %d (indirecto)\n", index, blockNum)
			return blockNum
		}
		//fmt.Printf("DEBUG: getBlockNumber(%d) = -1 (no asignado)\n", index)
		return -1
	}

	// === ESCRIBIR CONTENIDO COMPLETO EN TODOS LOS BLOQUES ===
	bytesWritten := int32(0)
	for blockIndex := 0; blockIndex < int(blocksNeeded); blockIndex++ {
		blockNum := getBlockNumber(int32(blockIndex))
		if blockNum == -1 {
			return fmt.Errorf("bloque %d no asignado correctamente", blockIndex)
		}

		// VERIFICAR QUE NO SE ESTÁ ESCRIBIENDO AL MISMO BLOQUE DOS VECES
		for prevIndex := 0; prevIndex < blockIndex; prevIndex++ {
			prevBlockNum := getBlockNumber(int32(prevIndex))
			if prevBlockNum == blockNum {
				return fmt.Errorf("ERROR CRÍTICO: Bloque %d está duplicado en posiciones %d y %d",
					blockNum, prevIndex, blockIndex)
			}
		}

		remainingBytes := contentSize - bytesWritten
		bytesToWrite := int32(64)
		if remainingBytes < bytesToWrite {
			bytesToWrite = remainingBytes
		}

		// Crear y llenar el bloque COMPLETAMENTE LIMPIO
		fileBlock := &bloques.BloqueArchivo{}
		for i := 0; i < 64; i++ {
			fileBlock.B_content[i] = 0
		}

		// Copiar el contenido correspondiente a este bloque
		if bytesToWrite > 0 {
			copy(fileBlock.B_content[:bytesToWrite], contentBytes[bytesWritten:bytesWritten+bytesToWrite])
		}
		/*
			// DEBUG: Mostrar qué se está escribiendo
			blockContent := string(fileBlock.B_content[:bytesToWrite])
			fmt.Printf("\n--- DEBUG BLOQUE %d (num: %d) ---\n", blockIndex, blockNum)
			fmt.Printf("Posición en contenido: %d-%d\n", bytesWritten, bytesWritten+bytesToWrite)
			fmt.Printf("Bytes a escribir: %d\n", bytesToWrite)
			fmt.Printf("Contenido del bloque:\n%s\n", blockContent)
			fmt.Printf("-------------------------------\n")
		*/
		// Escribir el bloque al disco
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque %d: %s", blockNum, err)
		}

		bytesWritten += bytesToWrite
		//fmt.Printf("✅ Bloque %d (físico #%d) escrito: %d bytes (total: %d/%d)\n",
		//	blockIndex, blockNum, bytesToWrite, bytesWritten, contentSize)
	}

	// === ACTUALIZAR SUPERBLOQUE ===
	err := utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	// === ACTUALIZAR INODO ===
	inode.SetSize(contentSize)
	inode.UpdateModificationTime()
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (inodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo: %s", err)
	}
	/*
			fmt.Printf("\n=== ESCRITURA COMPLETADA (MKGRP) ===\n")
			fmt.Printf("Total escrito: %d bytes en %d bloques\n", contentSize, blocksNeeded)
			iblockArrayFinal := inode.GetIblock()
			iblockSliceFinal := iblockArrayFinal[:]
			fmt.Printf("Bloques usados: %v\n", iblockSliceFinal[:blocksNeeded])
		fmt.Printf("====================================\n\n")
	*/
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

// writeContentToFile escribe contenido actualizado al archivo users.txt en EXT2
func writeContentToFile(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))

	// Obtener los bloques actuales del archivo
	blocks := inode.GetIblock()

	// Calcular cuántos bloques necesitamos y verificar límites
	blocksNeeded := (contentSize + 63) / 64 // Redondear hacia arriba (64 bytes por bloque)

	if blocksNeeded > 16 {
		return fmt.Errorf("el contenido es demasiado grande: necesita %d bloques, máximo 16", blocksNeeded)
	}

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

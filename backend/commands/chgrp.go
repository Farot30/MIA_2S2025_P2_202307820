package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	structuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
)

func CHGRP(args []string) {
	// verificar que root este logueado
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		fmt.Println("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	sessionInfo := session.GetCurrentSession()

	if sessionInfo.Username != "root" {
		fmt.Printf("Error: Solo el usuario root puede cambiar grupos\n")
		fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
		return
	}

	// parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	usarname := strings.Trim(params["user"], "\"")
	if usarname == "" {
		fmt.Println("Error: Falta el parámetro obligatorio -user")
		fmt.Println("Uso: chgrp -user=<nombre_usuario> -grp=<nombre_grupo>")
		return
	}

	if usarname == "root" {
		fmt.Println("Error: No se puede cambiar el grupo del usuario 'root'")
		return
	}

	groupName := strings.Trim(params["grp"], "\"")
	if groupName == "" {
		fmt.Println("Error: Debe especificar el nombre del grupo")
		fmt.Println("Uso: chgrp -user=<nombre_usuario> -grp=<nombre_grupo>")
		return
	}

	// obtener la partición montada
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

	if fsType == structuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		err = CambiarGrupoUsuario(mountedPartition, usarname, groupName)
	} else if fsType == structuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		err = CambiarGrupoUsuarioEXT3(mountedPartition, usarname, groupName)
	} else {
		fmt.Printf("Error: El sistema de archivos '%d' no es compatible con este comando\n", fsType)
		return
	}

	if err != nil {
		fmt.Printf("Error cambiando grupo: %s\n", err)
		return
	}

	fmt.Printf("Usuario '%s' ahora pertenece al grupo '%s'\n", usarname, groupName)
}

func CambiarGrupoUsuario(partition MountedPartition, username string, groupName string) error {
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

	if ext2fs.SuperBlock.S_magic != structuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no está formateado como EXT2")
	}

	// BUSCAR EL ARCHIVO users.txt
	usersInodeNum, err := findFileInRootDirectory(file, ext2fs, "users.txt")
	if err != nil {
		return fmt.Errorf("error buscando archivo users.txt: %s", err)
	}

	// Leer el inodo de users.txt
	usersInode := &structuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (usersInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// LEER CONTENIDO ACTUAL DEL ARCHIVO users.txt
	currentContent, err := readFileContentFromBlocks(file, ext2fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	fmt.Printf("DEBUG: Contenido actual de users.txt:\n%s\n", currentContent)

	// VERIFICAR QUE EL USUARIO EXISTE
	userLine, userExists := findUserInContent(currentContent, username)
	if !userExists {
		return fmt.Errorf("el usuario '%s' no existe", username)
	}

	// VERIFICAR QUE EL NUEVO GRUPO EXISTE Y ESTÁ ACTIVO
	if !groupExistsAndActive(currentContent, groupName) {
		return fmt.Errorf("el grupo '%s' no existe o está eliminado", groupName)
	}

	// CAMBIAR EL GRUPO DEL USUARIO
	newContent, err := changeUserGroup(currentContent, groupName, username, userLine)
	if err != nil {
		return fmt.Errorf("error cambiando grupo del usuario: %s", err)
	}

	// ESCRIBIR EL CONTENIDO ACTUALIZADO
	err = writeContentToFileUser(file, ext2fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	fmt.Printf("Grupo del usuario '%s' cambiado exitosamente de '%s' a '%s'\n",
		username, extractGroupFromUserLine(userLine), groupName)

	return nil
}

func CambiarGrupoUsuarioEXT3(partition MountedPartition, username string, groupName string) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT3 de la partición
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	if ext3fs.SuperBlock.S_magic != structuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no está formateado como EXT3")
	}

	// BUSCAR EL ARCHIVO users.txt
	usersInodeNum, err := findFileInRootDirectoryEXT3(file, ext3fs, "users.txt")
	if err != nil {
		return fmt.Errorf("error buscando archivo users.txt: %s", err)
	}

	// Leer el inodo de users.txt
	usersInode := &structuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (usersInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// LEER CONTENIDO ACTUAL DEL ARCHIVO users.txt
	currentContent, err := readFileContentFromBlocksEXT3(file, ext3fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	//fmt.Printf("DEBUG: Contenido actual de users.txt:\n%s\n", currentContent)

	// VERIFICAR QUE EL USUARIO EXISTE
	userLine, userExists := findUserInContent(currentContent, username)
	if !userExists {
		return fmt.Errorf("el usuario '%s' no existe", username)
	}

	// VERIFICAR QUE EL NUEVO GRUPO EXISTE Y ESTÁ ACTIVO
	if !groupExistsAndActive(currentContent, groupName) {
		return fmt.Errorf("el grupo '%s' no existe o está eliminado", groupName)
	}

	// CAMBIAR EL GRUPO DEL USUARIO
	newContent, err := changeUserGroup(currentContent, groupName, username, userLine)
	if err != nil {
		return fmt.Errorf("error cambiando grupo del usuario: %s", err)
	}

	// ESCRIBIR EL CONTENIDO ACTUALIZADO
	err = writeContentToFileUserEXT3(file, ext3fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	LogEXT3Operation(EstructuraExt3P2.OPERATION_CHGRP, "/users.txt", "-")

	fmt.Printf("Grupo del usuario '%s' cambiado exitosamente de '%s' a '%s'\n",
		username, extractGroupFromUserLine(userLine), groupName)

	return nil

}

// FUNCIÓN PARA BUSCAR UN USUARIO EN EL CONTENIDO
func findUserInContent(content, username string) (string, bool) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 4 {
			// Formato: ID,U,GRUPO,USUARIO,PASSWORD
			if parts[1] == "U" && parts[3] == username && parts[0] != "0" {
				return line, true // Usuario encontrado y no está eliminado
			}
		}
	}
	return "", false
}

// FUNCIÓN PARA CAMBIAR EL GRUPO DE UN USUARIO
func changeUserGroup(content, newGroup, username, oldUserLine string) (string, error) {
	lines := strings.Split(content, "\n")
	var newLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(oldUserLine) {
			// Esta es la línea del usuario que queremos cambiar
			parts := strings.Split(line, ",")
			if len(parts) >= 5 {
				// Formato: ID,U,USUARIO,GRUPO_VIEJO,PASSWORD
				oldGroup := parts[3]
				newUserLine := fmt.Sprintf("%s,U,%s,%s,%s", parts[0], newGroup, username, parts[4])
				newLines = append(newLines, newUserLine)

				fmt.Printf("Cambiando usuario: %s\n", username)
				fmt.Printf("   Grupo anterior: %s\n", oldGroup)
				fmt.Printf("   Grupo nuevo: %s\n", newGroup)
				fmt.Printf("   Línea anterior: %s\n", line)
				fmt.Printf("   Línea nueva: %s\n", newUserLine)
			} else {
				return "", fmt.Errorf("formato de línea de usuario inválido: %s", line)
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	return strings.Join(newLines, "\n"), nil
}

// FUNCIÓN PARA EXTRAER EL GRUPO DE UNA LÍNEA DE USUARIO
func extractGroupFromUserLine(userLine string) string {
	parts := strings.Split(userLine, ",")
	if len(parts) >= 4 {
		return parts[3] // El grupo está en la posición 3
	}
	return "desconocido"
}

// writeContentToFileUserEXT3 escribe contenido en EXT3 - VERSIÓN CORREGIDA
func writeContentToFileUserEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *estructuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))
	/*
		fmt.Printf("\n=== DEBUG ESCRITURA EXT3 ===\n")
		fmt.Printf("Contenido a escribir (%d bytes):\n%s\n", contentSize, content)
		fmt.Printf("============================\n\n")
	*/
	// Calcular bloques necesarios
	blocksNeeded := (contentSize + 63) / 64

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
		// Asignar el array devuelto por GetIblock a una variable
		iblockArray := inode.GetIblock()

		// Convertir el array en un slice
		iblockSlice := iblockArray[:]

		// Ahora puedes usar iblockSlice
		fmt.Printf("Bloques finales del inodo: %v\n", iblockSlice[:4])*/

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

			LogEXT3Operation(EstructuraExt3P2.OPERATION_EDIT, "/users.txt", "-")
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

	fmt.Printf("\n=== ESCRITURA COMPLETADA ===\n")
	fmt.Printf("Total escrito: %d bytes en %d bloques\n", contentSize, blocksNeeded)
	// Asignar el array devuelto por GetIblock a una variable
	//iblockArrays := inode.GetIblock()

	// Convertir el array en un slice
	//iblockSlices := iblockArrays[:]

	// Ahora puedes usar iblockSlice
	//fmt.Printf("Bloques usados: %v\n", iblockSlices[:blocksNeeded])
	fmt.Printf("============================\n\n")

	return nil
}

func updateBlockBitmapEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32, used bool) error {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return fmt.Errorf("error seeking block bitmap: %s", err)
	}

	var bitmapByte [1]byte
	_, err = file.Read(bitmapByte[:])
	if err != nil {
		return err
	}

	mask := byte(1 << (7 - bitIndex))
	if used {
		bitmapByte[0] |= mask // Marcar como usado
	} else {
		bitmapByte[0] &^= mask // Marcar como libre
	}

	_, err = file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return fmt.Errorf("error seeking block bitmap for write: %s", err)
	}

	_, err = file.Write(bitmapByte[:])
	return err
}

func markBlockAsUsedEXT3For(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	return updateBlockBitmapEXT3(file, ext3fs, blockNum, true)
}

func markBlockAsFreeEXT3For(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	return updateBlockBitmapEXT3(file, ext3fs, blockNum, false)
}
